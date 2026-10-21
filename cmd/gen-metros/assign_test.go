package main

import "testing"

func swapProject(f func(lat, lon float64) (float64, float64)) func() {
	old := project
	project = f
	return func() { project = old }
}

func TestAssignMatchesCountryAndPolygon(t *testing.T) {
	us := fua{id: 1, country: "us", geom: multiPolygon{{square(0, 0, 10)}}}
	us.minX, us.minY, us.maxX, us.maxY = us.geom.bbox()
	mx := fua{id: 2, country: "mx", geom: multiPolygon{{square(0, 0, 10)}}}
	mx.minX, mx.minY, mx.maxX, mx.maxY = mx.geom.bbox()
	// Places are given in degrees; to keep the test independent of the projection,
	// project is swapped for identity.
	defer swapProject(func(lat, lon float64) (float64, float64) { return lon, lat })()
	got := assign([]gnPlace{
		{name: "San Diego", country: "us", lat: 5, lon: 5, pop: 1_400_000},
		{name: "Tijuana", country: "mx", lat: 5, lon: 5, pop: 1_900_000},
		{name: "Nowhere", country: "us", lat: 50, lon: 50, pop: 20_000},
	}, []fua{us, mx})
	if len(got) != 2 || got[0].fuaID != 1 || got[1].fuaID != 2 {
		t.Fatalf("assign = %+v", got)
	}
}

func TestMetroNamesByPopulationWithOverride(t *testing.T) {
	as := []assigned{
		{gnPlace{name: "San Jose", country: "us", pop: 1_000_000}, 1255},
		{gnPlace{name: "San Francisco", country: "us", pop: 870_000}, 1255},
		{gnPlace{name: "Sacramento", country: "us", pop: 500_000}, 7},
		{gnPlace{name: "Folsom", country: "us", pop: 80_000}, 7},
	}
	got, err := metroNames(as, map[int]string{1255: "San Francisco Bay Area"})
	if err != nil {
		t.Fatal(err)
	}
	if got[1255] != "San Francisco Bay Area, US" || got[7] != "Sacramento, US" {
		t.Errorf("metroNames = %v", got)
	}
}

func TestMetroNamesDisambiguatesByAdmin1ThenFails(t *testing.T) {
	as := []assigned{
		{gnPlace{name: "Springfield", country: "us", admin1: "MA", pop: 150_000}, 1},
		{gnPlace{name: "Springfield", country: "us", admin1: "MO", pop: 160_000}, 2},
	}
	got, err := metroNames(as, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got[1] != "Springfield MA, US" || got[2] != "Springfield MO, US" {
		t.Errorf("metroNames = %v", got)
	}
	_, err = metroNames([]assigned{
		{gnPlace{name: "Valencia", country: "es", pop: 800_000}, 3},
		{gnPlace{name: "Valencia", country: "es", pop: 10_000}, 4},
	}, nil)
	if err == nil {
		t.Error("same-country collision without admin1 must fail")
	}
}
