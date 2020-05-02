// Copyright Krzesimir Nowak
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

type CombGen struct {
	n    int
	idxs []int
}

func NewCombGen(n int) *CombGen {
	return &CombGen{
		n:    n,
		idxs: nil,
	}
}

func NCombs(n int) uint64 {
	return (uint64)(1) << n
}

func (g *CombGen) Next() bool {
	if len(g.idxs) > g.n {
		return false
	}
	if g.idxs == nil {
		g.idxs = []int{}
		return true
	}
	i := len(g.idxs) - 1
	l := g.n - 1
	for i >= 0 {
		if g.idxs[i] < l {
			g.idxs[i]++
			for i2 := i + 1; i2 < len(g.idxs); i2++ {
				inc := i2 - i
				g.idxs[i2] = g.idxs[i] + inc
			}
			return true
		}
		i--
		l--
	}
	g.idxs = append(g.idxs, 0)
	if len(g.idxs) > g.n {
		return false
	}
	for i := range g.idxs {
		g.idxs[i] = i
	}
	return true
}

func (g *CombGen) Get() []int {
	return g.idxs
}
