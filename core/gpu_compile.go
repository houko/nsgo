//go:build !nsgo_noshader

package core

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/shader"
	"github.com/nsgo-dev/nsgo/internal/shaderir"
)

var gpuTextureCall = regexp.MustCompile(`\bTexture([0-3])At\s*\(`)

// CompileGPUKage is the core compiler entry point for package gpu.
func CompileGPUKage(source string) (*shaderir.Program, error) {
	if !strings.HasPrefix(strings.TrimSpace(source), "package ") {
		source = "package main\n" + source
	}
	textures := 0
	for _, match := range gpuTextureCall.FindAllStringSubmatch(source, -1) {
		textures = max(textures, int(match[1][0]-'0')+1)
	}
	var code strings.Builder
	code.WriteString(source)
	if textures != 0 {
		fmt.Fprintln(&code, "\n"+shader.InternalRegionBegin)
		for i := range textures {
			fmt.Fprintf(&code, "func Texture%[1]dAt(uv vec2) vec4 { return __texelAt(__t%[1]d, uv) }\n", i)
		}
		fmt.Fprintln(&code, shader.InternalRegionEnd)
	}
	program, err := shader.Compile([]byte(code.String()), "Vertex", "Fragment", textures)
	if err != nil {
		return nil, err
	}
	if program.VertexFunc.Block == nil || program.FragmentFunc.Block == nil {
		return nil, errors.New("a GPU program needs Vertex and Fragment entry points")
	}
	return program, nil
}
