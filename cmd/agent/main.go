// Package main is the health-dashboard agent binary.
// It collects system metrics (CPU, memory, disk) and ships them to the server.
// Full implementation in Task 3.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "agent: not yet implemented — see Task 3")
	os.Exit(1)
}
