package ce102m

import (
	"fmt"
	"testing"
	"time"
)

func TestDuration_UnmarshalTOML(t *testing.T) {
	var tests = []struct {
		s       string
		want    Duration
		wantErr bool
	}{
		{s: `10n`, want: Duration{tt: 10 * time.Nanosecond}},
		{s: `10ns`, want: Duration{tt: 10 * time.Nanosecond}},
		{s: `10u`, want: Duration{tt: 10 * time.Microsecond}},
		{s: `10µ`, want: Duration{tt: 10 * time.Microsecond}},
		{s: `10us`, want: Duration{tt: 10 * time.Microsecond}},
		{s: `10µs`, want: Duration{tt: 10 * time.Microsecond}},
		{s: `15ms`, want: Duration{tt: 15 * time.Millisecond}},
		{s: `100s`, want: Duration{tt: 100 * time.Second}},
		{s: `3`, want: Duration{tt: 3 * time.Second}},
		{s: `1000`, want: Duration{tt: 1000 * time.Second}},
		{s: `2m`, want: Duration{tt: 2 * time.Minute}},
		{s: `2mo`, want: Duration{monts: 2}},
		{s: `2h`, want: Duration{tt: 2 * time.Hour}},
		{s: `2d`, want: Duration{tt: 2 * 24 * time.Hour}},
		{s: `2w`, want: Duration{tt: 2 * 7 * 24 * time.Hour}},
		{s: `2y`, want: Duration{years: 2}},
		{s: `2y3h4s5us6ns`, want: Duration{years: 2,
			tt: 3*time.Hour + 4*time.Second +
				5*time.Microsecond + 6*time.Nanosecond}},
		{s: `1h30m`, want: Duration{tt: time.Hour + 30*time.Minute}},
		{s: `30ms3000u`, want: Duration{tt: 30*time.Millisecond + 3000*time.Microsecond}},
		{s: `-5s`, want: Duration{tt: -5 * time.Second}},
		{s: `-5m30s`, want: Duration{tt: -5*time.Minute - 30*time.Second}},
		{s: ``, want: Duration{}},
		{s: `3mm`, wantErr: true},
		{s: `3nm`, wantErr: true},
		{s: `w`, wantErr: true},
		{s: `ms`, wantErr: true},
		{s: `1.2w`, wantErr: true},
		{s: `10x`, wantErr: true},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			var got Duration
			err := got.UnmarshalTOML([]byte(tt.s))
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalTOML(%s) error = %v, wantErr %v", tt.s, err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if got != tt.want {
				t.Errorf("UnmarshalTOML(%s) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}

func TestDuration_Empty(t *testing.T) {
	var tests = []struct {
		d    Duration
		want bool
	}{
		{
			d:    Duration{},
			want: true,
		},
		{
			d:    Duration{tt: time.Second},
			want: false,
		},
		{
			d:    Duration{monts: 1},
			want: false,
		},
		{
			d:    Duration{years: 1},
			want: false,
		},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			got := tt.d.Empty()
			if got != tt.want {
				t.Errorf("Empty for %v returns %v", tt.d, got)
			}
		})
	}
}

func TestDuration_Until(t *testing.T) {
	tm := time.Now()
	d := Duration{tt: 1 * time.Minute}
	if d.Until(tm) <= 0 {
		t.Errorf("Until a minute after now failed.")
	}
	tm = tm.Add(-2 * time.Minute)

	if d.Until(tm) >= 0 {
		t.Errorf("Until 2 minutes past failed.")
	}
}

func TestTariffFliter(t *testing.T) {
	var tests = []struct {
		cfg     []int
		want    *tariffFilter
		wantErr bool
	}{
		{
			cfg:     []int{},
			want:    &tariffFilter{isAll: true},
			wantErr: false,
		},
		{
			cfg: []int{2},
			want: &tariffFilter{isSingle: true, minId: 2, arg: "2",
				mask: [maxTariffId]bool{false, true, false, false, false}},
			wantErr: false,
		},
		{
			cfg: []int{2, 2, 2},
			want: &tariffFilter{isSingle: true, minId: 2, arg: "2",
				mask: [maxTariffId]bool{false, true, false, false, false}},
			wantErr: false,
		},
		{
			cfg: []int{2, 3},
			want: &tariffFilter{minId: 2, arg: "2,2",
				mask: [maxTariffId]bool{false, true, true, false, false}},
			wantErr: false,
		},
		{
			cfg: []int{1, 3, 5},
			want: &tariffFilter{minId: 1, arg: "1,5",
				mask: [maxTariffId]bool{true, false, true, false, true}},
			wantErr: false,
		},
		{
			cfg: []int{1, 2, 3, 4, 5},
			want: &tariffFilter{isAll: true, minId: 1, arg: "1,5",
				mask: [maxTariffId]bool{true, true, true, true, true}},
			wantErr: false,
		},
		{
			cfg:     []int{-1},
			wantErr: true,
		},
		{
			cfg:     []int{0},
			wantErr: true,
		},
		{
			cfg:     []int{6},
			wantErr: true,
		},
		{
			cfg:     []int{1, 2, 6, 3},
			wantErr: true,
		},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			got, err := newTariffFilter(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("newTariffFliter(%v) error = %v, wantErr %v", tt.cfg, err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if *got != *tt.want {
				t.Errorf("newTariffFliter(%v) = %v, want %v", tt.cfg, got, tt.want)
			}
		})
	}
}
