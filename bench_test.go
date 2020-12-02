package main

import (
	"fmt"
	"os"
	"testing"
	"uk.ac.bris.cs/gameoflife/gol"
)

func benchmarkGol (threads int, b *testing.B) {
	b.Run(fmt.Sprint(threads), func(b *testing.B) {
		os.Stdout = nil // Disable all program output apart from benchmark results
		params := gol.Params{
			Turns:       100,
			Threads:     threads,
			ImageWidth:  512,
			ImageHeight: 512,
			Rejoin:      false,
			Engine:      "52.201.227.232:8030",
		}
		for i := 0; i < b.N; i++ {
			events := make(chan gol.Event, 1000)
			b.StartTimer()
			gol.Run(params, events, nil)
			for range events {}
			b.StopTimer()
		}
	})
}

//func BenchmarkThreads1(b *testing.B) { benchmarkGol(1, b) }
//func BenchmarkThreads2(b *testing.B) { benchmarkGol(2, b) }
//func BenchmarkThreads3(b *testing.B) { benchmarkGol(3, b) }
//func BenchmarkThreads4(b *testing.B) { benchmarkGol(4, b) }
//func BenchmarkThreads5(b *testing.B) { benchmarkGol(5, b) }
//func BenchmarkThreads6(b *testing.B) { benchmarkGol(6, b) }
//func BenchmarkThreads7(b *testing.B) { benchmarkGol(7, b) }
//func BenchmarkThreads8(b *testing.B) { benchmarkGol(8, b) }
//func BenchmarkThreads9(b *testing.B) { benchmarkGol(9, b) }
//func BenchmarkThreads10(b *testing.B) { benchmarkGol(10, b) }
//func BenchmarkThreads11(b *testing.B) { benchmarkGol(11, b) }
//func BenchmarkThreads12(b *testing.B) { benchmarkGol(12, b) }
//func BenchmarkThreads13(b *testing.B) { benchmarkGol(13, b) }
//func BenchmarkThreads14(b *testing.B) { benchmarkGol(14, b) }
//func BenchmarkThreads15(b *testing.B) { benchmarkGol(15, b) }
//func BenchmarkThreads16(b *testing.B) { benchmarkGol(16, b) }

// Run with "go test -bench . bench_test.go
