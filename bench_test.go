package main

import (
	"fmt"
	"os"
	"testing"
	"uk.ac.bris.cs/gameoflife/gol"
)

func BenchmarkGol (b *testing.B) {
	for thread := 1; thread <= 8; thread++ {
		b.Run(fmt.Sprint(thread), func(b *testing.B) {
			os.Stdout = nil // Disable all program output apart from benchmark results
			params := gol.Params{
				Turns:       100,
				Threads:     thread,
				ImageWidth:  5120,
				ImageHeight: 5120,
				Engine:      "3.239.113.143:8030",
			}
			for i := 0; i < b.N; i++ {
				events := make(chan gol.Event)
				b.StartTimer()
				gol.Run(params, events, nil)
				for range events {
				}
				b.StopTimer()
			}
		})
	}
}

// Run with "go test -bench . bench_test.go
