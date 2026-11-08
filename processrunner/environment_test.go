package processrunner

import (
	"runtime"
	"testing"
)

const environmentProbeVariable = "PROCESSRUNNER_TEST_ENV_VALUE"

func TestEnvironmentMapOverridesInheritedValues(t *testing.T) {
	t.Setenv(environmentProbeVariable, "inherited")
	for _, test := range []struct {
		name  string
		value string
		want  string
	}{
		{name: "replace", value: "configured", want: "configured"},
		{name: "empty value", value: "", want: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			invocation := helperInvocation("environment")
			invocation.Environment[environmentProbeVariable] = test.value
			result, err := Run(t.Context(), invocation, RunOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if got := string(result.Output); got != test.want {
				t.Fatalf("environment value = %q, want %q", got, test.want)
			}
		})
	}
}

func TestMergeEnvironmentUsesPlatformNameComparison(t *testing.T) {
	got := mergeEnvironment([]string{"Path=inherited"}, map[string]string{"PATH": "configured"})
	if runtime.GOOS == "windows" {
		if len(got) != 1 || got[0] != "PATH=configured" {
			t.Fatalf("merged environment = %#v, want one case-insensitive override", got)
		}
		return
	}
	if len(got) != 2 || got[0] != "Path=inherited" || got[1] != "PATH=configured" {
		t.Fatalf("merged environment = %#v, want distinct case-sensitive variables", got)
	}
}

func TestStartRejectsInvalidEnvironmentMap(t *testing.T) {
	for _, test := range []struct {
		name  string
		key   string
		value string
	}{
		{name: "empty name", key: "", value: "value"},
		{name: "equals in name", key: "BAD=NAME", value: "value"},
		{name: "nul in name", key: "BAD\x00NAME", value: "value"},
		{name: "nul in value", key: "NAME", value: "bad\x00value"},
	} {
		t.Run(test.name, func(t *testing.T) {
			invocation := helperInvocation("output")
			invocation.Environment[test.key] = test.value
			if handle, err := Start(t.Context(), invocation, RunOptions{}); err == nil || handle != nil {
				t.Fatalf("Start() = %v, %v; want invalid environment error", handle, err)
			}
		})
	}
}
