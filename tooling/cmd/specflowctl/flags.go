package main

import "strings"

// repeatedString is a repeatable string flag value.
type repeatedString []string

func (r *repeatedString) String() string {
	return strings.Join(*r, ",")
}

func (r *repeatedString) Set(v string) error {
	*r = append(*r, v)
	return nil
}
