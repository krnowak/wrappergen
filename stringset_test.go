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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStringSet(t *testing.T) {
	s1 := StringSet{}
	assert.Equal(t, 0, s1.Len())
	assert.False(t, s1.Has("foo"))
	s1.Add("foo")
	assert.Equal(t, 1, s1.Len())
	assert.True(t, s1.Has("foo"))
	s1.Add("foo")
	assert.Equal(t, 1, s1.Len())
	assert.True(t, s1.Has("foo"))

	slice := []string{"foo", "bar", "baz", "bar", "baz"}
	s1.AddSlice(slice)
	assert.True(t, s1.Has("foo"))
	assert.True(t, s1.Has("bar"))
	assert.True(t, s1.Has("baz"))
	assert.Equal(t, 3, s1.Len())

	s2 := StringSet{}
	s2.AddSome("foo", "bar", "quux")

	s3 := StringSet{}
	s3.AddSet(s1)
	s3.AddSet(s2)
	assert.Equal(t, 4, s3.Len())
	assert.True(t, s3.Has("foo"))
	assert.True(t, s3.Has("bar"))
	assert.True(t, s3.Has("baz"))
	assert.True(t, s3.Has("quux"))

	slice = []string{"a", "b", "c"}
	s4 := StringSet{}
	s4.AddSlice(slice)

	s5 := StringSet{}
	s5.AddSome("b", "c", "d")

	s45Diff := s4.Diff(s5)
	s54Diff := s5.Diff(s4)

	assert.Equal(t, 1, s45Diff.Len())
	assert.True(t, s45Diff.Has("a"))
	assert.Equal(t, 1, s54Diff.Len())
	assert.True(t, s54Diff.Has("d"))

	s4s := s4.ToSlice()
	assert.Equal(t, slice, s4s)
}
