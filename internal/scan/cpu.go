package scan

import "runtime"

func numCPU() int { return runtime.NumCPU() }
