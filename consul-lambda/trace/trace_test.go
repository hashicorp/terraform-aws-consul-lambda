// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package trace_test

import (
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/terraform-aws-consul-lambda/consul-lambda/trace"
)

func TestTraceHCLog(t *testing.T) {
	trace.Enabled(false)
	require.False(t, trace.IsEnabled())
	trace.Enabled(true)
	require.True(t, trace.IsEnabled())
	trace.SetLogger(trace.NewHCLog(hclog.Default(), hclog.Info))
	trace.SetTag("trace")

	Func1()
	Func2()
	Both()
	trace.SetTag("")
	Func1()
}

func Func1() {
	trace.Enter()
	time.Sleep(time.Millisecond)
	trace.Exit()
}

func Func2() {
	trace.Enter()
	time.Sleep(time.Millisecond)
	trace.Exit()
}

func Both() {
	trace.Enter()
	Func1()
	Func2()
	trace.Exit()
}
