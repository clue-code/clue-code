//go:build !test

package main

// runHooksFireTest is a stub for non-test builds. The real implementation
// lives in hooks_test_helper.go and is compiled only with -tags=test.
func runHooksFireTest(args []string) {
	// unreachable: hooksFireTestEnabled is false in non-test builds,
	// so runHooks() exits before reaching this call.
	panic("runHooksFireTest called in non-test build")
}
