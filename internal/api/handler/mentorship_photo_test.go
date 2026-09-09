package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/candidate/headshot"
	"github.com/strelov1/freehire/internal/engage/mentorship"
)

func TestToMentorResponseCarriesShowPhoto(t *testing.T) {
	p := mentorship.Profile{ShowPhoto: true}
	if !toMentorResponse(p).ShowPhoto {
		t.Error("show_photo = false in public response, want true")
	}
	if toMentorResponse(mentorship.Profile{ShowPhoto: false}).ShowPhoto {
		t.Error("show_photo = true in public response, want false")
	}
}

func TestToOwnAndModeratorMentorResponseCarryShowPhoto(t *testing.T) {
	p := mentorship.Profile{ShowPhoto: true}
	if !toOwnMentorResponse(p).ShowPhoto {
		t.Error("owner view: show_photo = false, want true")
	}
	if !toModeratorMentorResponse(p).ShowPhoto {
		t.Error("moderator view: show_photo = false, want true")
	}
}

// The four fixtures below reuse fakePhotoBlobs/fakePhotoRepo/newFakePhotoBlobs from
// photo_test.go rather than redeclaring them.

func TestMentorPhoto_ApprovedOptedInWithStoredHeadshot_ServesBytes(t *testing.T) {
	blobs := newFakePhotoBlobs()
	blobs.objs["headshots/7"] = []byte("jpeg-bytes")
	repo := &fakePhotoRepo{key: "headshots/7", set: true}
	store := headshot.New(blobs, repo)

	data, err := mentorPhoto(context.Background(), mentorship.Profile{UserID: 7, ShowPhoto: true}, store)
	if err != nil {
		t.Fatalf("mentorPhoto: %v", err)
	}
	if string(data) != "jpeg-bytes" {
		t.Errorf("data = %q, want the stored bytes", data)
	}
}

func TestMentorPhoto_NotOptedIn_NotFoundEvenWithAStoredHeadshot(t *testing.T) {
	blobs := newFakePhotoBlobs()
	blobs.objs["headshots/7"] = []byte("jpeg-bytes")
	repo := &fakePhotoRepo{key: "headshots/7", set: true}
	store := headshot.New(blobs, repo)

	_, err := mentorPhoto(context.Background(), mentorship.Profile{UserID: 7, ShowPhoto: false}, store)
	if !errors.Is(err, errMentorPhotoNotFound) {
		t.Errorf("err = %v, want errMentorPhotoNotFound", err)
	}
}

func TestMentorPhoto_OptedInButNoStoredHeadshot_NotFound(t *testing.T) {
	store := headshot.New(newFakePhotoBlobs(), &fakePhotoRepo{})

	_, err := mentorPhoto(context.Background(), mentorship.Profile{UserID: 7, ShowPhoto: true}, store)
	if !errors.Is(err, errMentorPhotoNotFound) {
		t.Errorf("err = %v, want errMentorPhotoNotFound", err)
	}
}

func TestMentorPhoto_StorageUnconfigured_NotFound(t *testing.T) {
	store := headshot.New(nil, &fakePhotoRepo{})

	_, err := mentorPhoto(context.Background(), mentorship.Profile{UserID: 7, ShowPhoto: true}, store)
	if !errors.Is(err, errMentorPhotoNotFound) {
		t.Errorf("err = %v, want errMentorPhotoNotFound", err)
	}
}
