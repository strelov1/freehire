package main

import (
	"reflect"
	"testing"
)

func TestToSeedEntries(t *testing.T) {
	got := toSeedEntries(map[string]seedEntry{
		"edzz.fa.em3.oraclecloud.com/CX_6001": {
			Board: "edzz.fa.em3.oraclecloud.com/CX_6001", Company: "University of Birmingham",
		},
		"mcgill.wd3.myworkdayjobs.com/careers": {
			Board: "mcgill.wd3.myworkdayjobs.com/careers", Company: "McGill University",
		},
	})
	want := []seedEntry{
		// sorted by board so the emitted seed is deterministic
		{Board: "edzz.fa.em3.oraclecloud.com/CX_6001", Company: "University of Birmingham"},
		{Board: "mcgill.wd3.myworkdayjobs.com/careers", Company: "McGill University"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}
