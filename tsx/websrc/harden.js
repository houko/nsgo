(function (g) {
	"use strict";
	const refuse = function () { throw new EvalError("nsgo: code is not made from strings here"); };
	const functionPrototypes = [function () {}, async function () {}, function* () {}].map(Object.getPrototypeOf);
	// Goja does not parse async generators. Include the intrinsic on engines
	// that support the syntax before removing the Function constructors.
	try {
		const make = g.Function("return (async function* () {})");
		functionPrototypes.push(Object.getPrototypeOf(make()));
	} catch (err) {
		if (!(err instanceof SyntaxError) && !(err instanceof EvalError)) throw err;
	}
	const codeConstructors = functionPrototypes.map(proto => proto.constructor);
	Object.defineProperty(refuse, "prototype", { value: g.Function.prototype });
	for (const proto of functionPrototypes) {
		Object.defineProperty(proto, "constructor", { value: refuse, writable: false, configurable: false });
	}
	delete g.eval;
	Object.defineProperty(g, "Function", { value: refuse, writable: false, configurable: false });

	const frozen = new Set();
	function freezeChain(value) {
		while (value != null && !frozen.has(value)) {
			frozen.add(value);
			Object.freeze(value);
			value = Object.getPrototypeOf(value);
		}
	}
	function freezeConstructor(C) {
		if (typeof C !== "function") return;
		freezeChain(C);
		freezeChain(C.prototype);
	}

	for (const C of codeConstructors) {
		freezeConstructor(C);
		freezeChain(C.prototype.prototype);
	}
	for (const name of ["Object", "Array", "Function", "String", "Number", "Boolean", "Symbol", "BigInt",
		"Promise", "Map", "Set", "WeakMap", "WeakSet", "WeakRef", "FinalizationRegistry", "Date", "RegExp",
		"Error", "AggregateError", "EvalError", "GoError", "RangeError", "ReferenceError", "SyntaxError", "TypeError",
		"URIError", "SuppressedError", "ArrayBuffer", "SharedArrayBuffer", "DataView", "Proxy", "Iterator",
		"AsyncIterator", "DisposableStack", "AsyncDisposableStack"]) {
		freezeConstructor(g[name]);
	}
	// Discover concrete typed arrays by their shared constructor, including
	// arrays that only a newer engine exposes.
	const typedArray = Object.getPrototypeOf(g.Uint8Array);
	for (const name of Object.getOwnPropertyNames(g)) {
		const d = Object.getOwnPropertyDescriptor(g, name);
		const value = d && d.value;
		if (typeof value === "function" && Object.getPrototypeOf(value) === typedArray) {
			freezeConstructor(value);
		}
	}
	for (const name of ["JSON", "Math", "Reflect", "Atomics", "Intl", "Temporal"]) {
		const namespace = g[name];
		if (namespace == null) continue;
		if (name === "Intl" || name === "Temporal") {
			for (const descriptor of Object.values(Object.getOwnPropertyDescriptors(namespace))) {
				freezeConstructor(descriptor.value);
			}
		}
		freezeChain(namespace);
	}
})
