# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

locals {
  selector = provider::ctrlplane::normalize_selector(<<-EOT
    (
      resource.version == 'ctrlplane.dev/kubernetes/cluster/v1' &&
      resource.metadata['kubernetes/status'] == 'running'
    )
  EOT
  )
}
