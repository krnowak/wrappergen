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
	"sort"
)

type StringSet map[string]struct{}

func (s StringSet) Add(str string) {
	s[str] = struct{}{}
}

func (s StringSet) AddSome(strs ...string) {
	s.AddSlice(strs)
}

func (s StringSet) AddSet(other StringSet) {
	for str := range other {
		s.Add(str)
	}
}

func (s StringSet) AddSlice(other []string) {
	for _, str := range other {
		s.Add(str)
	}
}

func (s StringSet) Has(str string) bool {
	_, ok := s[str]
	return ok
}

func (s StringSet) Len() int {
	return len(s)
}

func (s StringSet) Diff(other StringSet) StringSet {
	diff := StringSet{}
	for str := range s {
		if !other.Has(str) {
			diff.Add(str)
		}
	}
	return diff
}

func (s StringSet) ToSlice() []string {
	slice := make([]string, 0, len(s))
	for str := range s {
		slice = append(slice, str)
	}
	sort.Strings(slice)
	return slice
}
