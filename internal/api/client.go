// Copyright IBM Corp. 2021, 2026

package api

import (
	"context"
	"errors"
	"math/rand"
	"net/http"
	"strings"
	"time"

	apiv1connect "buf.build/gen/go/ctrlplane/ctrlplane/connectrpc/go/ctrlplane/api/v1/apiv1connect"
	apiv1 "buf.build/gen/go/ctrlplane/ctrlplane/protocolbuffers/go/ctrlplane/api/v1"
	connect "connectrpc.com/connect"
	"github.com/google/uuid"
)

const (
	retryMaxAttempts = 4
	retryBaseDelay   = 200 * time.Millisecond
	retryMaxDelay    = 5 * time.Second
)

// apiKeyInterceptor returns a Connect interceptor that attaches the API key to
// every outbound request via the X-API-Key header. The key is never placed in
// the URL.
func apiKeyInterceptor(apiKey string) connect.Interceptor {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			req.Header().Set("X-API-Key", apiKey)
			return next(ctx, req)
		}
	})
}

// isRetryable reports whether a Connect error is a transient condition where
// replaying the request is reasonable: the server was unreachable
// (Unavailable) or rate-limited the caller (ResourceExhausted). Both imply the
// request did not take effect, so retrying is side-effect safe.
func isRetryable(err error) bool {
	switch connect.CodeOf(err) {
	case connect.CodeUnavailable, connect.CodeResourceExhausted:
		return true
	default:
		return false
	}
}

// isIdempotentProcedure reports whether an RPC is safe to replay. Create* RPCs
// are not: a dropped response after the server already created the entity would
// turn a retry into a duplicate create. Everything else here (Get/List/Upsert/
// Update/Delete/Set/Link/Unlink) is idempotent.
func isIdempotentProcedure(procedure string) bool {
	return !strings.Contains(procedure, "/Create")
}

// retryInterceptor retries transient failures on idempotent RPCs with a capped,
// jittered exponential backoff. It is a transport-resilience measure (engine
// restart, load-balancer blip, rate limit) and is unrelated to read-after-write
// consistency — it replays the same call, it does not poll.
func retryInterceptor() connect.Interceptor {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			idempotent := isIdempotentProcedure(req.Spec().Procedure)
			delay := retryBaseDelay

			var resp connect.AnyResponse
			var err error
			for attempt := 1; ; attempt++ {
				resp, err = next(ctx, req)
				if err == nil || attempt >= retryMaxAttempts || !idempotent || !isRetryable(err) {
					return resp, err
				}

				// Full jitter: sleep in [delay, 2*delay).
				sleep := delay + time.Duration(rand.Int63n(int64(delay)+1))
				timer := time.NewTimer(sleep)
				select {
				case <-ctx.Done():
					timer.Stop()
					return resp, err
				case <-timer.C:
				}
				if delay *= 2; delay > retryMaxDelay {
					delay = retryMaxDelay
				}
			}
		}
	})
}

// WorkspaceClient bundles the Connect service clients together with the
// resolved workspace ID. Resources and data sources type-assert the provider
// data to *WorkspaceClient and call the appropriate service.
type WorkspaceClient struct {
	ID  uuid.UUID
	Url string

	Resource       apiv1connect.ResourceServiceClient
	System         apiv1connect.SystemServiceClient
	Deployment     apiv1connect.DeploymentServiceClient
	Policy         apiv1connect.PolicyServiceClient
	Job            apiv1connect.JobServiceClient
	Workflow       apiv1connect.WorkflowServiceClient
	VariableSet    apiv1connect.VariableSetServiceClient
	Workspace      apiv1connect.WorkspaceServiceClient
	Secret         apiv1connect.SecretServiceClient
	SecretProvider apiv1connect.SecretProviderServiceClient
}

// WorkspaceID returns the workspace ID as a string, the form expected by the
// Connect request messages.
func (c *WorkspaceClient) WorkspaceID() string {
	return c.ID.String()
}

// NewWorkspaceClient builds the Connect service clients against the engine's
// base URL and resolves the workspace (slug or UUID) to its ID. Connect serves
// RPCs at the host root, so no path suffix is appended to the endpoint.
func NewWorkspaceClient(endpoint string, apiKey string, workspace string) (*WorkspaceClient, error) {
	baseURL := strings.TrimSuffix(endpoint, "/")

	httpClient := &http.Client{}
	// retryInterceptor is listed first so it is the outermost interceptor: each
	// retry re-runs apiKeyInterceptor and re-sends with the auth header set.
	opts := connect.WithInterceptors(retryInterceptor(), apiKeyInterceptor(apiKey))

	c := &WorkspaceClient{
		Url:            endpoint,
		Resource:       apiv1connect.NewResourceServiceClient(httpClient, baseURL, opts),
		System:         apiv1connect.NewSystemServiceClient(httpClient, baseURL, opts),
		Deployment:     apiv1connect.NewDeploymentServiceClient(httpClient, baseURL, opts),
		Policy:         apiv1connect.NewPolicyServiceClient(httpClient, baseURL, opts),
		Job:            apiv1connect.NewJobServiceClient(httpClient, baseURL, opts),
		Workflow:       apiv1connect.NewWorkflowServiceClient(httpClient, baseURL, opts),
		VariableSet:    apiv1connect.NewVariableSetServiceClient(httpClient, baseURL, opts),
		Workspace:      apiv1connect.NewWorkspaceServiceClient(httpClient, baseURL, opts),
		Secret:         apiv1connect.NewSecretServiceClient(httpClient, baseURL, opts),
		SecretProvider: apiv1connect.NewSecretProviderServiceClient(httpClient, baseURL, opts),
	}

	id, err := c.resolveWorkspaceID(context.Background(), workspace)
	if err != nil {
		return nil, err
	}
	c.ID = id

	return c, nil
}

// resolveWorkspaceID accepts either a UUID or a slug and returns the workspace
// UUID, mirroring the behavior of the previous OpenAPI client.
func (c *WorkspaceClient) resolveWorkspaceID(ctx context.Context, workspace string) (uuid.UUID, error) {
	if id, err := uuid.Parse(workspace); err == nil {
		return id, nil
	}

	resp, err := c.Workspace.GetWorkspaceBySlug(ctx, connect.NewRequest(&apiv1.GetWorkspaceBySlugRequest{
		Slug: workspace,
	}))
	if err != nil {
		return uuid.Nil, err
	}

	ws := resp.Msg
	if ws == nil || ws.GetId() == "" {
		return uuid.Nil, errors.New("workspace not found")
	}

	id, err := uuid.Parse(ws.GetId())
	if err != nil {
		return uuid.Nil, err
	}
	return id, nil
}
