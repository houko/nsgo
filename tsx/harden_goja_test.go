//go:build !js

package tsx

import (
	"testing"

	"github.com/nsgo-dev/nsgo/tsx/internal/js"
)

// Discover constructors in a pristine goja runtime so an engine upgrade
// adding a global constructor also brings it under this check.
func TestHardenCoversRuntimeConstructors(t *testing.T) {
	r := &runtime{vm: js.New()}
	check, err := r.vm.RunString(`(function () {
		const constructors = [];
		for (const name of Object.getOwnPropertyNames(globalThis)) {
			const C = globalThis[name];
			if (typeof C === "function" && (C.prototype || name === "Proxy")) constructors.push([name, C]);
		}
		return () => constructors.flatMap(([name, C]) => {
			const missing = [];
			if (!Object.isFrozen(C)) missing.push(name);
			if (C.prototype && !Object.isFrozen(C.prototype)) missing.push(name + ".prototype");
			return missing;
		}).join(", ");
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	r.harden()
	inspect, _ := js.AssertFunction(check)
	missing, err := inspect(js.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	if got := missing.String(); got != "" {
		t.Fatalf("unfrozen intrinsics: %s", got)
	}
}
