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
	if len(got) != 3 || got[0].fuaID != 1 || got[1].fuaID != 2 || got[2].fuaID != noFUA {
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
	got, err := metroNames(as, nil, map[int]string{1255: "San Francisco Bay Area"})
	if err != nil {
		t.Fatal(err)
	}
	if got[1255] != "San Francisco Bay Area (US)" || got[7] != "Sacramento (US)" {
		t.Errorf("metroNames = %v", got)
	}
}

func TestMetroNamesDominantNamesakeKeepsPlainName(t *testing.T) {
	as := []assigned{
		{gnPlace{name: "Springfield", country: "us", admin1: "MA", pop: 150_000}, 1},
		{gnPlace{name: "Springfield", country: "us", admin1: "MO", pop: 160_000}, 2},
	}
	got, err := metroNames(as, map[int]float64{1: 700_000, 2: 400_000}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got[1] != "Springfield (US)" || got[2] != "Springfield MO (US)" {
		t.Errorf("metroNames = %v", got)
	}
}

func TestMetroNamesQualifiesNonUSByAdmin1Name(t *testing.T) {
	as := []assigned{
		{gnPlace{name: "Goiânia", country: "br", region: "Goiás"}, 1},
		{gnPlace{name: "Planaltina", country: "br", region: "Goiás", pop: 5}, 2},
		{gnPlace{name: "Planaltina", country: "br", region: "Distrito Federal", pop: 9}, 3},
	}
	got, err := metroNames(as, map[int]float64{2: 10, 3: 20}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got[3] != "Planaltina (BR)" || got[2] != "Planaltina Goiás (BR)" {
		t.Errorf("metroNames = %v", got)
	}
}

func TestMetroNamesFallsThroughToAdmin2(t *testing.T) {
	as := []assigned{
		{gnPlace{name: "Udaipur", country: "in", region: "Udaipur", district: "Udaipur", pop: 9}, 1},
		{gnPlace{name: "Udaipur", country: "in", region: "Udaipur", district: "Jaipur", pop: 8}, 2},
	}
	got, err := metroNames(as, map[int]float64{1: 2, 2: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got[1] != "Udaipur (IN)" || got[2] != "Udaipur Jaipur (IN)" {
		t.Errorf("metroNames = %v", got)
	}
	// A qualifier equal to the name is skipped too.
	as = []assigned{
		{gnPlace{name: "Una", country: "in", region: "Una", district: "Hamirpur", pop: 9}, 1},
		{gnPlace{name: "Una", country: "in", region: "Una", district: "Una", pop: 8}, 2},
	}
	got, err = metroNames(as, map[int]float64{1: 1, 2: 2}, nil)
	if err != nil || got[2] != "Una (IN)" || got[1] != "Una Hamirpur (IN)" {
		t.Errorf("metroNames = %v, %v", got, err)
	}
}

func TestMetroNamesSurvivingCollisionFails(t *testing.T) {
	_, err := metroNames([]assigned{
		{gnPlace{name: "Valencia", country: "es", pop: 800_000}, 3},
		{gnPlace{name: "Valencia", country: "es", pop: 10_000}, 4},
	}, nil, nil)
	if err == nil {
		t.Error("same-country collision without a qualifier must fail")
	}
}

func TestMetroNamesQualifiedValueCollidingWithExistingFallsThrough(t *testing.T) {
	as := []assigned{
		{gnPlace{name: "Foo", country: "xx", region: "Bar", district: "Baz", pop: 9}, 1},
		{gnPlace{name: "Foo", country: "xx", region: "Bar", district: "Baz", pop: 8}, 2},
		{gnPlace{name: "Foo Bar", country: "xx", pop: 7}, 3},
	}
	got, err := metroNames(as, map[int]float64{1: 3, 2: 2, 3: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got[2] != "Foo Baz (XX)" || got[3] != "Foo Bar (XX)" {
		t.Errorf("metroNames = %v", got)
	}
	// With no admin2 to fall to, the same shape must fail rather than emit a duplicate.
	as[1].place.district, as[0].place.district = "", ""
	if _, err := metroNames(as, map[int]float64{1: 3, 2: 2, 3: 1}, nil); err == nil {
		t.Error("post-qualification collision must be caught")
	}
}

func TestMetroNamesRejectsComma(t *testing.T) {
	as := []assigned{{gnPlace{name: "A, B", country: "xx", pop: 1}, 1}}
	if _, err := metroNames(as, map[int]float64{1: 1}, nil); err == nil {
		t.Error("a value containing a comma must be an error")
	}
}
