// Copyright IBM Corp. 2022, 2025
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"flag"
	"os"
	"strings"
	"testing"

	testsuite "github.com/hashicorp/terraform-aws-consul-lambda/test/acceptance/framework/suite"
)

var suite testsuite.Suite

func ensureAcceptanceTestTimeoutDefault() {
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "-test.timeout=") || strings.HasPrefix(arg, "-timeout=") {
			return
		}
	}

	if timeoutFlag := flag.Lookup("test.timeout"); timeoutFlag != nil {
		_ = timeoutFlag.Value.Set("45m")
	}
}

func TestMain(m *testing.M) {
	ensureAcceptanceTestTimeoutDefault()
	suite = testsuite.NewSuite(m)
	os.Exit(suite.Run())
}
