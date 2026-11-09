package video

import "testing"

func TestClockAndTypes(t *testing.T) {
	for ms, want := range map[int64]string{0: "00:00:00.000", 7000: "00:00:07.000", 3723456: "01:02:03.456"} {
		if got := Clock(ms); got != want {
			t.Errorf("Clock(%d) = %q, want %q", ms, got, want)
		}
	}
	for name, want := range map[string]string{"cftv.MP4": "video/mp4", "a.mov": "video/quicktime", "x.webm": "video/webm", "photo.png": "", "noext": ""} {
		if got := contentType(name); got != want {
			t.Errorf("contentType(%q) = %q, want %q", name, got, want)
		}
	}
}
