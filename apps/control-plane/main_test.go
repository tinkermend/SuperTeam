package main_test

import (
	"fmt"
	"testing"
	"github.com/superteam/control-plane/internal/api/gen"
)

func TestGenError(t *testing.T) {
	_ = gen.ErrorResponse{Message: "test"}
	fmt.Println("OK")
}
