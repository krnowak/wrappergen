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

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNCombs(t *testing.T) {
	type testcase struct {
		n     int
		ncomb uint64
	}
	tcs := []testcase{
		{
			n:     0,
			ncomb: 1,
		},
		{
			n:     1,
			ncomb: 2,
		},
		{
			n:     2,
			ncomb: 4,
		},
		{
			n:     3,
			ncomb: 8,
		},
		{
			n:     4,
			ncomb: 16,
		},
		{
			n:     5,
			ncomb: 32,
		},
		{
			n:     6,
			ncomb: 64,
		},
		{
			n:     7,
			ncomb: 128,
		},
		{
			n:     8,
			ncomb: 256,
		},
		{
			n:     9,
			ncomb: 512,
		},
		{
			n:     10,
			ncomb: 1024,
		},
	}
	for _, tc := range tcs {
		got := NCombs(tc.n)
		assert.Equal(t, tc.ncomb, got, "NCombs(%d)", tc.n)
	}
}

func TestCombGen(t *testing.T) {
	type testcase struct {
		n     int
		combs []string
	}
	testcases := []testcase{
		{
			n:     0,
			combs: []string{""},
		},
		{
			n:     1,
			combs: []string{"", "0"},
		},
		{
			n:     2,
			combs: []string{"", "0", "1", "01"},
		},
		{
			n:     3,
			combs: []string{"", "0", "1", "2", "01", "02", "12", "012"},
		},
		{
			n:     4,
			combs: []string{"", "0", "1", "2", "3", "01", "02", "03", "12", "13", "23", "012", "013", "023", "123", "0123"},
		},
		{
			n:     5,
			combs: []string{"", "0", "1", "2", "3", "4", "01", "02", "03", "04", "12", "13", "14", "23", "24", "34", "012", "013", "014", "023", "024", "034", "123", "124", "134", "234", "0123", "0124", "0134", "0234", "1234", "01234"},
		},
	}
	for _, tc := range testcases {
		expectedSet := StringSet{}
		expectedSet.AddSlice(tc.combs)
		require.Len(t, tc.combs, expectedSet.Len(), "bug in testcase")
		cg := NewCombGen(tc.n)
		strs := make([]string, 0, NCombs(tc.n))
		for cg.Next() {
			idxs := cg.Get()
			sb := strings.Builder{}
			for _, idx := range idxs {
				sb.WriteString(strconv.FormatInt((int64)(idx), 10))
			}
			strs = append(strs, sb.String())
		}
		failed := !assert.Len(t, strs, len(tc.combs))
		gotSet := StringSet{}
		gotSet.AddSlice(strs)
		missing := expectedSet.Diff(gotSet).ToSlice()
		extra := gotSet.Diff(expectedSet)
		if !assert.NotEmpty(t, missing, "missing elements from generated combinations: %#v", missing) {
			failed = true
		}
		if !assert.NotEmpty(t, extra, "extra elements in generated combinations: %#v", extra) {
			failed = true
		}
		if failed {
			// do not bother with checking the ordering if
			// we failed the test case
			continue
		}
		// generated and expected combinations now have the
		// same length and the same elements, check the
		// ordering
		for idx := 0; idx < len(strs); idx++ {
			assert.Equal(t, tc.combs[idx], strs[idx], "bad value at index %d", idx)
		}
	}
}
