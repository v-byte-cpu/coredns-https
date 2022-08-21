package https

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRandomPolicy(t *testing.T) {
	tests := []struct {
		name     string
		poolLen  int
		expected [][]int
	}{
		{
			name:    "NegativeLength",
			poolLen: -1,
			expected: [][]int{
				nil,
			},
		},
		{
			name:    "ZeroElements",
			poolLen: 0,
			expected: [][]int{
				nil,
				nil,
			},
		},
		{
			name:    "OneElement",
			poolLen: 1,
			expected: [][]int{
				[]int{0},
				[]int{0},
			},
		},
		{
			name:    "TwoElements",
			poolLen: 2,
		},
		{
			name:    "ThreeElements",
			poolLen: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRandomPolicy()
			if tt.poolLen < 2 {
				for i, expected := range tt.expected {
					result := r.List(len(expected))
					require.Equal(t, expected, result, "iteration %d", i)
				}
			} else {
				result := r.List(tt.poolLen)
				require.Equal(t, tt.poolLen, len(result))
				// verify all elements
				visited := make([]bool, tt.poolLen)
				for i := 0; i < tt.poolLen; i++ {
					require.Less(t, result[i], tt.poolLen)
					require.False(t, visited[result[i]], "element %d is duplicated", result[i])
					visited[result[i]] = true
				}
			}
		})
	}
}

func TestRoundRobinPolicy(t *testing.T) {
	tests := []struct {
		name     string
		poolLen  int
		expected [][]int
	}{
		{
			name:    "NegativeLength",
			poolLen: -1,
			expected: [][]int{
				nil,
			},
		},
		{
			name:    "ZeroElements",
			poolLen: 0,
			expected: [][]int{
				nil,
				nil,
			},
		},
		{
			name:    "OneElement",
			poolLen: 1,
			expected: [][]int{
				[]int{0},
				[]int{0},
			},
		},
		{
			name:    "TwoElements",
			poolLen: 2,
			expected: [][]int{
				[]int{0, 1},
				[]int{1, 0},
				[]int{0, 1},
			},
		},
		{
			name:    "ThreeElements",
			poolLen: 3,
			expected: [][]int{
				[]int{0, 1, 2},
				[]int{1, 2, 0},
				[]int{2, 0, 1},
				[]int{0, 1, 2},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRoundRobinPolicy()
			for i, expected := range tt.expected {
				result := r.List(len(expected))
				require.Equal(t, expected, result, "iteration %d", i)
			}
		})
	}
}

func TestSequentialPolicy(t *testing.T) {
	tests := []struct {
		name     string
		poolLen  int
		expected [][]int
	}{
		{
			name:    "NegativeLength",
			poolLen: -1,
			expected: [][]int{
				nil,
			},
		},
		{
			name:    "ZeroElements",
			poolLen: 0,
			expected: [][]int{
				nil,
				nil,
			},
		},
		{
			name:    "OneElement",
			poolLen: 1,
			expected: [][]int{
				[]int{0},
				[]int{0},
			},
		},
		{
			name:    "TwoElements",
			poolLen: 2,
			expected: [][]int{
				[]int{0, 1},
				[]int{0, 1},
			},
		},
		{
			name:    "ThreeElements",
			poolLen: 3,
			expected: [][]int{
				[]int{0, 1, 2},
				[]int{0, 1, 2},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newSequentialPolicy()
			for i, expected := range tt.expected {
				result := p.List(len(expected))
				require.Equal(t, expected, result, "iteration %d", i)
			}
		})
	}
}
