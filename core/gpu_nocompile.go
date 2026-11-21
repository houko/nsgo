//go:build nsgo_noshader

package core

import (
	"errors"

	"github.com/nsgo-dev/nsgo/internal/shaderir"
)

// CompileGPUKage is package gpu's compiler entry point on this build.
func CompileGPUKage(string) (*shaderir.Program, error) {
	return nil, errors.New("this build has no GPU shader compiler")
}
