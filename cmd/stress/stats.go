package main

import (
	"fmt"
	"sort"
	"sync/atomic"
	"time"
)

type Stats struct {
	name      string
	total     atomic.Int64
	success   atomic.Int64
	fail      atomic.Int64
	durations []time.Duration
	start     time.Time
}

func NewStats(name string) *Stats {
	return &Stats{
		name:  name,
		start: time.Now(),
	}
}

func (s *Stats) Record(success bool, d time.Duration) {
	s.total.Add(1)
	if success {
		s.success.Add(1)
	} else {
		s.fail.Add(1)
	}
	s.durations = append(s.durations, d)
}

func (s *Stats) SuccessCount() int64 {
	return s.success.Load()
}

func (s *Stats) FailCount() int64 {
	return s.fail.Load()
}

func (s *Stats) TotalCount() int64 {
	return s.total.Load()
}

func (s *Stats) Report() {
	elapsed := time.Since(s.start)
	total := s.total.Load()
	success := s.success.Load()
	fail := s.fail.Load()

	var successRate float64
	if total > 0 {
		successRate = float64(success) / float64(total) * 100
	}

	var qps float64
	if elapsed.Seconds() > 0 {
		qps = float64(total) / elapsed.Seconds()
	}

	fmt.Printf("\n=== %s结果 ===\n", s.name)
	fmt.Printf("总请求数:   %d\n", total)
	fmt.Printf("成功数:     %d\n", success)
	fmt.Printf("失败数:     %d\n", fail)
	fmt.Printf("成功率:     %.2f%%\n", successRate)
	fmt.Printf("总耗时:     %s\n", elapsed.Round(time.Millisecond))
	fmt.Printf("QPS:        %.0f\n", qps)

	if len(s.durations) > 0 {
		sorted := make([]time.Duration, len(s.durations))
		copy(sorted, s.durations)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

		var sum time.Duration
		for _, d := range sorted {
			sum += d
		}
		avg := sum / time.Duration(len(sorted))

		fmt.Printf("平均耗时:   %s\n", avg.Round(time.Millisecond))
		fmt.Printf("P50:        %s\n", percentile(sorted, 50))
		fmt.Printf("P95:        %s\n", percentile(sorted, 95))
		fmt.Printf("P99:        %s\n", percentile(sorted, 99))
	}
}

func percentile(sorted []time.Duration, p int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(p) / 100 * float64(len(sorted)))
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx].Round(time.Millisecond)
}
