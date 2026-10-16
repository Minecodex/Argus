package presentation

import (
	"math"
	"sort"
)

// sampleWindow only selects original observations. Endpoints, finite extrema
// and both sides of every finite/missing transition are mandatory: dropping a
// gap marker would make the renderer draw observations that never existed.
// An empty preview explicitly means these guarantees exceed the point budget.
func sampleWindow(points []any, budget int) []any {
	if len(points) <= budget {
		return points
	}
	keep := map[int]bool{0: true, len(points) - 1: true}
	values := make([]float64, len(points))
	valid := make([]bool, len(points))
	low, high := -1, -1
	for i, point := range points {
		values[i] = sampleNumber(point)
		valid[i] = !math.IsNaN(values[i]) && !math.IsInf(values[i], 0)
		if i > 0 && valid[i] != valid[i-1] {
			keep[i-1], keep[i] = true, true
		}
		if !valid[i] {
			continue
		}
		if low < 0 || values[i] < values[low] {
			low = i
		}
		if high < 0 || values[i] > values[high] {
			high = i
		}
	}
	if low >= 0 {
		keep[low], keep[high] = true, true
	}
	if len(keep) > budget {
		return []any{}
	}
	// Spend the remaining quota on local extrema across the entire window.
	buckets := (budget - len(keep)) / 2
	for bucket := 0; bucket < buckets; bucket++ {
		start, end := len(points)*bucket/buckets, len(points)*(bucket+1)/buckets
		low, high := -1, -1
		for i := start; i < end; i++ {
			if !valid[i] {
				continue
			}
			if low < 0 || values[i] < values[low] {
				low = i
			}
			if high < 0 || values[i] > values[high] {
				high = i
			}
		}
		if low >= 0 {
			keep[low], keep[high] = true, true
		}
	}
	// Flat buckets may select the same point. Fill the largest remaining gap
	// with an actual sample, preserving chronological order and a strict bound.
	indices := func() []int {
		out := make([]int, 0, len(keep))
		for i := range keep {
			out = append(out, i)
		}
		sort.Ints(out)
		return out
	}
	for len(keep) < budget {
		order := indices()
		left, gap := 0, 0
		for i := 1; i < len(order); i++ {
			if distance := order[i] - order[i-1]; distance > gap {
				left, gap = order[i-1], distance
			}
		}
		if gap <= 1 {
			break
		}
		keep[left+gap/2] = true
	}
	out := make([]any, 0, len(keep))
	for _, i := range indices() {
		out = append(out, points[i])
	}
	return out
}
