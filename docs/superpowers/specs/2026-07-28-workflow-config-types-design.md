# Preserve Workflow Job-Agent Configuration Types

## Problem

`ctrlplane_workflow.job_agent.config` is declared as `map(string)`. The provider
therefore converts every configuration value to a string before sending it to
Ctrlplane. This prevents workflows from supplying typed values such as the
numeric GitHub Actions `workflowId`.

Existing workflows, including the shared-tenancy Argo Workflows definitions,
use string-only configuration maps and must continue to plan and apply without
configuration changes or state churn.

## Design

Change `ctrlplane_workflow.job_agent.config` from a string map to a dynamic
attribute. A dynamic attribute reflects the Ctrlplane API's
`map<string, any>` contract and preserves primitive and nested Terraform types.

The public Terraform syntax remains unchanged:

```hcl
config = {
  repo       = "deployments"
  workflowId = 321561144
  ref        = "main"
}
```

String-only configurations remain valid:

```hcl
config = {
  name     = "instance-tests"
  template = file("workflow.yaml")
}
```

The workflow model will store `config` as `types.Dynamic`. Conversion to the
API will recursively translate the dynamic Terraform value into Go values.
Conversion from the API will recursively construct the corresponding dynamic
Terraform value. The implementation will reuse the existing dynamic
configuration conversion behavior used by `ctrlplane_resource`, extracting a
shared helper only where that avoids duplication.

## State Compatibility

Changing an attribute's type changes the resource state schema. Increment the
`ctrlplane_workflow` schema version from 0 to 1 and implement a version-0 state
upgrader.

The upgrader will:

1. Decode the prior workflow state using the original `map(string)` job-agent
   configuration schema.
2. Preserve every existing string key and value.
3. Write each configuration map as the underlying value of the new dynamic
   attribute.
4. Preserve workflow IDs, names, slugs, inputs, agent names, references, and
   selectors unchanged.

No changes are required in existing Terraform configurations. Refreshing or
planning an existing shared-tenancy workflow must not produce a configuration
diff caused by this migration.

## Validation and Errors

The dynamic value must resolve to an object or map. Null and unknown values are
handled consistently with the current empty-configuration behavior. Any other
top-level type returns a provider diagnostic explaining that workflow
job-agent configuration must be an object.

Conversion errors must be returned rather than silently replacing the
configuration with an empty map.

## Tests

Add focused unit tests covering:

- Existing string-only job-agent configurations round-trip unchanged.
- A numeric `workflowId` remains numeric through Terraform-to-API conversion.
- Mixed strings, numbers, booleans, lists, and nested objects round-trip.
- Null and empty configuration behavior.
- Invalid scalar top-level configuration returns an error.
- Version-0 state containing `map(string)` upgrades to version 1 without
  changing values.

Run the full provider test suite after the focused tests.

## Rollout

Publish a provider version containing the schema migration. Existing users can
upgrade normally; Terraform invokes the state upgrader before planning.
After the provider version is available, the managed-spec workflow can keep
`workflowId` as a numeric Terraform value. The shared-tenancy workflow files do
not need to change.
