# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

schema = "1"

project "terraform-aws-consul-lambda" {
  // the team key is not used by CRT currently
  team = "consul-ecs"
  slack {
    notification_channel = "C01J8QV0EF8" # team-consul-ecs
  }
  github {
    organization = "hashicorp"
    repository = "terraform-aws-consul-lambda"
    release_branches = [
      "main",
      "release/0.1.x",
    ]
  }
}

event "merge" {
  // "entrypoint" to use if build is not run automatically
  // i.e. send "merge" complete signal to orchestrator to trigger build
}

event "build" {
  depends = ["merge"]
  action "build" {
    organization = "hashicorp"
    repository = "terraform-aws-consul-lambda"
    workflow = "build"
  }
}

event "prepare" {
  depends = ["build"]
  action "prepare" {
    organization = "hashicorp"
    repository   = "crt-workflows-common"
    workflow     = "prepare"
    depends      = ["build"]
  }

  notification {
    on = "fail"
  }
}

## These are promotion and post-publish events
## they should be added to the end of the file after the verify event stanza.

event "trigger-staging" {
// This event is dispatched by the bob trigger-promotion command
// and is required - do not delete.
}

event "promote-staging" {
  depends = ["trigger-staging"]
  action "promote-staging" {
    organization = "hashicorp"
    repository = "crt-workflows-common"
    workflow = "promote-staging"
    config = "release-metadata.hcl"
  }

  notification {
    on = "always"
  }
}

event "promote-staging-docker" {
  depends = ["promote-staging"]
  action "promote-staging-docker" {
    organization = "hashicorp"
    repository = "crt-workflows-common"
    workflow = "promote-staging-docker"
  }

  notification {
    on = "always"
  }
}

event "trigger-production" {
// This event is dispatched by the bob trigger-promotion command
// and is required - do not delete.
}

event "promote-production" {
  depends = ["trigger-production"]
  action "promote-production" {
    organization = "hashicorp"
    repository = "crt-workflows-common"
    workflow = "promote-production"
  }

  notification {
    on = "always"
  }
}

event "promote-production-docker" {
  depends = ["promote-production"]
  action "promote-production-docker" {
    organization = "hashicorp"
    repository = "crt-workflows-common"
    workflow = "promote-production-docker"
  }

  notification {
    on = "always"
  }
}
