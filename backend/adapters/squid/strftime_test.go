// in-package test: strftime の書式を読む手順 (compileStrftime、scan、civil) は unexported であるため。
package squid

import (
	"errors"
	"testing"
	"time"
)

// 期待する文字列は glibc 2.39 の strftime (C locale、TZ=XST-9) が 2026-03-05 07:04:09 +0900 に対して出した値である。
func TestStrftimeScanReadsGlibcOutput(t *testing.T) {
	const wall = "2026-03-05T07:04:09Z"
	cases := []struct {
		format, value string
		want          string
		wantOffset    int
	}{
		{"%d/%b/%Y:%H:%M:%S %z", "05/Mar/2026:07:04:09 +0900", "2026-03-05T07:04:09+09:00", 9 * 3600},
		{"%d/%b/%Y:%H:%M:%S %z", "05/Mar/2026:07:04:09 -0930", "2026-03-05T07:04:09-09:30", -(9*3600 + 30*60)},
		{"%Y/%m/%d %H:%M:%S", "2026/03/05 07:04:09", wall, 0},
		{"%c", "Thu Mar  5 07:04:09 2026", wall, 0},
		{"%Ec", "Thu Mar  5 07:04:09 2026", wall, 0},
		{"%a, %e %B %Y %T", "Thu,  5 March 2026 07:04:09", wall, 0},
		{"%A %D %r", "Thursday 03/05/26 07:04:09 AM", wall, 0},
		{"%A %D %r", "Thursday 03/05/26 07:04:09 PM", "2026-03-05T19:04:09Z", 0},
		{"%F %R:%S", "2026-03-05 07:04:09", wall, 0},
		{"%x %X", "03/05/26 07:04:09", wall, 0},
		{"%Y-%j %k:%M:%S", "2026-064  7:04:09", wall, 0},
		{"%C%y-%m-%d %l:%M:%S %P", "2026-03-05  7:04:09 am", wall, 0},
		{"%C%y-%m-%d %l:%M:%S %P", "2026-03-05  7:04:09 pm", "2026-03-05T19:04:09Z", 0},
		{"%^p %I:%M:%S %Y-%m-%d", "AM 07:04:09 2026-03-05", wall, 0},
		{"%^p %I:%M:%S %Y-%m-%d", "PM 12:04:09 2026-03-05", "2026-03-05T12:04:09Z", 0},
		{"%^p %I:%M:%S %Y-%m-%d", "AM 12:04:09 2026-03-05", "2026-03-05T00:04:09Z", 0},
		{"%y%m%d%H%M%S", "260305070409", wall, 0},
		{"%y%m%d%H%M%S", "680305070409", "2068-03-05T07:04:09Z", 0},
		{"%y%m%d%H%M%S", "690305070409", "1969-03-05T07:04:09Z", 0},
		{"%s", "1772661849", "2026-03-04T22:04:09Z", 0},
		{"%s", "-5", "1969-12-31T23:59:55Z", 0},
		{"%s %z", "1772661849 +0900", "2026-03-05T07:04:09+09:00", 9 * 3600},
		{"%G-W%V-%u %U %W %g %Y-%m-%d %H:%M:%S", "2026-W10-4 09 09 26 2026-03-05 07:04:09", wall, 0},
		{"%h %d %Y %H:%M:%S %Z", "Mar 05 2026 07:04:09 XST", wall, 0},
		{"%-d/%-m/%Y %-H:%-M:%-S", "5/3/2026 7:4:9", wall, 0},
		{"%_d %_m %Y %_H:%M:%S", " 5  3 2026  7:04:09", wall, 0},
		{"%0e %^b %Y %H:%M:%S", "05 MAR 2026 07:04:09", wall, 0},
		{"%5d-%_5m-%-3Y %H:%M:%S", "00005-    3-2026 07:04:09", wall, 0},
		{"%-3d/%m/%Y %H:%M:%S", "  5/03/2026 07:04:09", wall, 0},
		{"%3u %w %6Y-%m-%d %H:%M:%S", "004 4 002026-03-05 07:04:09", wall, 0},
		{"%Ey/%Om/%Od %OH:%OM:%OS", "26/03/05 07:04:09", wall, 0},
		{"%#a %^B %d %Y %H:%M:%S%%", "THU MARCH 05 2026 07:04:09%", wall, 0},
		{"%-j %Y %H:%M:%S", "64 2026 07:04:09", wall, 0},
		{"%Y-%m-%d %H:%M:%S %j %a %u", "2024-02-29 07:04:09 060 Thu 4", "2024-02-29T07:04:09Z", 0},
	}
	for _, tc := range cases {
		t.Run(tc.format+"/"+tc.value, func(t *testing.T) {
			format, err := compileStrftime(tc.format)
			if err != nil {
				t.Fatalf("compileStrftime: %v", err)
			}
			const prefix = "a ["
			raw := prefix + tc.value + "] 200 GET"
			end, clock, problem := format.scan(raw, len(prefix))
			if problem != nil {
				t.Fatalf("scan: problem at %d: %s", problem.offset, problem.expected)
			}
			if want := len(prefix) + len(tc.value); end != want {
				t.Fatalf("end = %d, want %d", end, want)
			}
			got, err := clock.civil()
			if err != nil {
				t.Fatalf("civil: %v", err)
			}
			if got.Format(time.RFC3339) != tc.want {
				t.Fatalf("civil = %s, want %s", got.Format(time.RFC3339), tc.want)
			}
			if _, offset := got.Zone(); offset != tc.wantOffset {
				t.Fatalf("zone offset = %d, want %d", offset, tc.wantOffset)
			}
		})
	}
}

func TestStrftimeFormatProperties(t *testing.T) {
	cases := []struct {
		format                      string
		fullDateTime, offset, epoch bool
	}{
		{"%d/%b/%Y:%H:%M:%S %z", true, true, false},
		{"%Y/%m/%d %H:%M:%S", true, false, false},
		{"%s", true, false, true},
		{"%Y-%j %H:%M:%S", true, false, false},
		{"%c", true, false, false},
		{"%r %F", true, false, false},
		{"%H:%M:%S", false, false, false},
		{"%Y-%m-%d %H:%M", false, false, false},
		{"%Y-%m %H:%M:%S", false, false, false},
		{"%m-%d %H:%M:%S %z", false, true, false},
		{"%Y-%m-%d %I:%M:%S", false, false, false},
		{"%C-%m-%d %H:%M:%S", false, false, false},
		{"%G-W%V-%u %H:%M:%S", false, false, false},
	}
	for _, tc := range cases {
		format, err := compileStrftime(tc.format)
		if err != nil {
			t.Fatalf("compileStrftime(%q): %v", tc.format, err)
		}
		if format.fullDateTime() != tc.fullDateTime || format.carriesOffset() != tc.offset ||
			format.carriesEpoch() != tc.epoch {
			t.Errorf("%q: fullDateTime=%v carriesOffset=%v carriesEpoch=%v, want %v %v %v", tc.format,
				format.fullDateTime(), format.carriesOffset(), format.carriesEpoch(),
				tc.fullDateTime, tc.offset, tc.epoch)
		}
	}
}

func TestWallClockCivilRejectsInconsistentValues(t *testing.T) {
	cases := []struct{ name, format, value string }{
		{"month 13", "%Y-%m-%d %H:%M:%S", "2026-13-05 07:04:09"},
		{"month 0", "%Y-%m-%d %H:%M:%S", "2026-00-05 07:04:09"},
		{"February 30", "%Y-%m-%d %H:%M:%S", "2024-02-30 07:04:09"},
		{"February 29 in a common year", "%Y-%m-%d %H:%M:%S", "2026-02-29 07:04:09"},
		{"day 0", "%Y-%m-%d %H:%M:%S", "2026-03-00 07:04:09"},
		{"hour 24", "%Y-%m-%d %H:%M:%S", "2026-03-05 24:04:09"},
		{"minute 60", "%Y-%m-%d %H:%M:%S", "2026-03-05 07:60:09"},
		{"second 60", "%Y-%m-%d %H:%M:%S", "2026-03-05 07:04:60"},
		{"weekday name", "%a %Y-%m-%d %H:%M:%S", "Fri 2026-03-05 07:04:09"},
		{"full weekday name", "%A %Y-%m-%d %H:%M:%S", "Friday 2026-03-05 07:04:09"},
		{"ISO weekday", "%u %Y-%m-%d %H:%M:%S", "5 2026-03-05 07:04:09"},
		{"ISO weekday 8", "%u %Y-%m-%d %H:%M:%S", "8 2026-03-02 07:04:09"},
		{"weekday number", "%w %Y-%m-%d %H:%M:%S", "7 2026-03-01 07:04:09"},
		{"day of the year", "%j %Y-%m-%d %H:%M:%S", "065 2026-03-05 07:04:09"},
		{"day of the year 366 in a common year", "%Y-%j %H:%M:%S", "2026-366 07:04:09"},
		{"day of the year 0", "%Y-%j %H:%M:%S", "2026-000 07:04:09"},
		{"12-hour clock without AM or PM", "%Y-%m-%d %I:%M:%S", "2026-03-05 07:04:09"},
		{"12-hour clock with 24-hour clock and no AM or PM", "%Y-%m-%d %H %I:%M:%S", "2026-03-05 07 07:04:09"},
		{"12-hour clock hour 13", "%Y-%m-%d %I:%M:%S %p", "2026-03-05 13:04:09 PM"},
		{"12-hour clock hour 0", "%Y-%m-%d %I:%M:%S %p", "2026-03-05 00:04:09 AM"},
		{"AM with an afternoon hour", "%Y-%m-%d %H:%M:%S %p", "2026-03-05 19:04:09 AM"},
		{"offset hour 24", "%Y-%m-%d %H:%M:%S %z", "2026-03-05 07:04:09 +2400"},
		{"offset minute 60", "%Y-%m-%d %H:%M:%S %z", "2026-03-05 07:04:09 +0060"},
		{"repeated component", "%d %Y-%m-%d %H:%M:%S", "06 2026-03-05 07:04:09"},
		{"century and year", "%C %Y-%m-%d %H:%M:%S", "19 2026-03-05 07:04:09"},
		{"year within the century and year", "%y %Y-%m-%d %H:%M:%S", "25 2026-03-05 07:04:09"},
		{"year within the century over 99", "%3y-%m-%d %H:%M:%S", "126-03-05 07:04:09"},
		{"no seconds", "%Y-%m-%d %H:%M", "2026-03-05 07:04"},
		{"no date", "%H:%M:%S", "07:04:09"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			format, err := compileStrftime(tc.format)
			if err != nil {
				t.Fatalf("compileStrftime: %v", err)
			}
			end, clock, problem := format.scan(tc.value, 0)
			if problem != nil || end != len(tc.value) {
				t.Fatalf("scan: end=%d problem=%v", end, problem)
			}
			if got, err := clock.civil(); err == nil {
				t.Fatalf("civil = %s, want an error", got.Format(time.RFC3339))
			}
		})
	}
}

func TestCompileStrftimeRejectsUnreadableFormats(t *testing.T) {
	cases := []struct {
		format     string
		wantOffset int
		wantToken  string
	}{
		{"%Y%n%m", 2, "%n"},
		{"%Y%t%m", 2, "%t"},
		{"%Y\t%m", 2, "\t"},
		{"%H:%M\n", 5, "\n"},
		{"%H:%M\r", 5, "\r"},
		{"%Y-%q", 3, "%q"},
		{"%Y-%+4Y", 3, "%+"},
		{"%H:%M%", 5, "%"},
		{"%H:%M%-_", 5, "%-_"},
		{"%10b", 0, "%10b"},
		{"%3p", 0, "%3p"},
		{"%10c", 0, "%10c"},
		{"%5%", 0, "%5%"},
		{"%12s", 0, "%12s"},
		{"%_z", 0, "%_z"},
		{"%-z", 0, "%-z"},
		{"%5z", 0, "%5z"},
		{"%Ed", 0, "%Ed"},
		{"%OY", 0, "%OY"},
		{"%O5d", 0, "%O5"},
		{"%EOy", 0, "%EO"},
		{"%99999999999999999999d", 0, "%99999999999999999999d"},
	}
	for _, tc := range cases {
		_, err := compileStrftime(tc.format)
		var formatErr *LogFormatError
		if !errors.As(err, &formatErr) {
			t.Errorf("compileStrftime(%q) error = %v, want *LogFormatError", tc.format, err)
			continue
		}
		if formatErr.Offset != tc.wantOffset || formatErr.Token != tc.wantToken {
			t.Errorf("compileStrftime(%q) = offset %d token %q, want offset %d token %q",
				tc.format, formatErr.Offset, formatErr.Token, tc.wantOffset, tc.wantToken)
		}
	}
}

func TestStrftimeScanReportsMismatchedByte(t *testing.T) {
	cases := []struct {
		format, raw string
		wantOffset  int
	}{
		{"%Y-%m-%d", "x 2026/03/05", 6},
		{"%d/%b/%Y", "x 5/Mar/2026", 3},
		{"%d/%b/%Y", "x 05/Mzr/2026", 5},
		{"%e %H", "x  x 07", 3},
		{"%e %H", "x    07", 3},
		{"%-d/%m", "x /03", 2},
		{"%H:%M:%S %z", "x 07:04:09 +09", 14},
		{"%H:%M:%S %z", "x 07:04:09 0900", 11},
		{"%s", "x -", 3},
		{"%s", "x abc", 2},
		{"[%Z]", "x []", 3},
		{"%p", "x XM", 2},
		{"%Y-%m-%d", "x 2026-03-0", 11},
	}
	for _, tc := range cases {
		format, err := compileStrftime(tc.format)
		if err != nil {
			t.Fatalf("compileStrftime(%q): %v", tc.format, err)
		}
		_, _, problem := format.scan(tc.raw, 2)
		if problem == nil {
			t.Errorf("scan(%q, %q) = no problem, want one at %d", tc.format, tc.raw, tc.wantOffset)
			continue
		}
		if problem.offset != tc.wantOffset || problem.expected == "" {
			t.Errorf("scan(%q, %q) problem = %d %q, want offset %d", tc.format, tc.raw,
				problem.offset, problem.expected, tc.wantOffset)
		}
	}
}

func TestStrftimeZoneNameStopsAtFollowingLiteral(t *testing.T) {
	format, err := compileStrftime("%Y-%m-%d %H:%M:%S %Z-x")
	if err != nil {
		t.Fatalf("compileStrftime: %v", err)
	}
	const value = "2026-03-05 07:04:09 UTC+9-x"
	end, _, problem := format.scan(value+" rest", 0)
	if problem != nil {
		t.Fatalf("scan: problem at %d: %s", problem.offset, problem.expected)
	}
	if end != len(value) {
		t.Fatalf("end = %d, want %d", end, len(value))
	}
}
