package core_test

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// Base64 の文字列と、復号後の文字列。**原資料の値を写していない。**
//
// 復号後の文字列は本 test が決めたコマンド行で、path は C:\tmp の下、対象は sample.txt で
// ある。Base64 の文字列は、その文字列を UTF-16LE で符号化して作った。作り方は
// python3 -c "import base64; base64.b64encode('...'.encode('utf-16-le'))" である。
// 期待値は復号後の文字列の側であり、実装の出力を転記していない。
const (
	encodedWriteOutput = "VwByAGkAdABlAC0ATwB1AHQAcAB1AHQAIABlAHgAYQBtAHAAbABlAA=="
	decodedWriteOutput = "Write-Output example"

	encodedGetItem = "RwBlAHQALQBJAHQAZQBtACAAQwA6AFwAdABtAHAAXABzAGEAbQBwAGwAZQAuAHQAeAB0AA=="
	decodedGetItem = `Get-Item C:\tmp\sample.txt`
)

// 符号化されていないのに Base64 として読めてしまう文字列。**原資料の値を写していない。**
//
// どちらも Base64 の文字だけから成り、長さが 4 の倍数で、UTF-16LE として復号すると
// 制御文字を含まない CJK の文字列になる。**制御文字の有無だけの検査を通る。**
// 受理条件を ASCII の印字可能文字に限ることで、この 2 つを復号の対象から外す。
const (
	// 24 文字ちょうどの、Base64 の文字だけから成る英単語。
	wordInTheBase64Alphabet = "ExampleContainerNameAbcd"
	// 32 文字の 16 進。hash の文字列と同じ形である。値は 0 から f を並べた値である。
	hexadecimalLikeAHash = "0123456789abcdef0123456789abcdef"
	// 制御文字を含む文字列を UTF-16LE で符号化した文字列。
	encodedWithControlCharacters = "QQABAEIAAgBDAAMARAAEAEUABQBGAAYARwAHAA=="
)

// switch に続く文字列を復号し、残りの文字列を原資料の文字列のまま並べた行を返す。
func TestDecodeEncodedCommandLineDecodesTheSwitchArgument(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		commandLine string
		want        string
	}{
		{
			name:        "full switch name",
			commandLine: "-Sta -Nop -Window Hidden -EncodedCommand " + encodedGetItem,
			want:        "-Sta -Nop -Window Hidden -EncodedCommand " + decodedGetItem,
		},
		{
			name:        "abbreviated switch name",
			commandLine: "-Sta -Nop -Window Hidden -enc " + encodedGetItem,
			want:        "-Sta -Nop -Window Hidden -enc " + decodedGetItem,
		},
		{
			name:        "slash prefix",
			commandLine: "/EncodedCommand " + encodedWriteOutput,
			want:        "/EncodedCommand " + decodedWriteOutput,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			got := core.DecodeEncodedCommandLine(testCase.commandLine)

			if !got.Encoded || !got.Decoded {
				t.Fatalf("encoded = %t and decoded = %t, want both true", got.Encoded, got.Decoded)
			}
			if got.CommandLine != testCase.want {
				t.Errorf("commandLine = %q, want %q", got.CommandLine, testCase.want)
			}
		})
	}
}

// switch を伴わない Base64 の文字列も、ASCII の読める文字列へ復号できれば置き換える。
func TestDecodeEncodedCommandLineDecodesAStandaloneLexeme(t *testing.T) {
	commandLine := "-WindowStyle Hidden -c $enc = '" + encodedGetItem + "';"
	want := "-WindowStyle Hidden -c $enc = '" + decodedGetItem + "';"

	got := core.DecodeEncodedCommandLine(commandLine)

	if !got.Encoded || !got.Decoded {
		t.Fatalf("encoded = %t and decoded = %t, want both true", got.Encoded, got.Decoded)
	}
	if got.CommandLine != want {
		t.Errorf("commandLine = %q, want %q", got.CommandLine, want)
	}
}

// 1 本の行に符号化された文字列が 2 つあるとき、両方を復号した行を返す。
func TestDecodeEncodedCommandLineDecodesEveryLexeme(t *testing.T) {
	commandLine := "-enc " + encodedWriteOutput + " ; " + encodedGetItem
	want := "-enc " + decodedWriteOutput + " ; " + decodedGetItem

	got := core.DecodeEncodedCommandLine(commandLine)

	if got.CommandLine != want {
		t.Errorf("commandLine = %q, want %q", got.CommandLine, want)
	}
}

// **符号化されていない文字列を復号しない。**
//
// 2 つの文字列はどちらも Base64 として復号でき、復号結果に制御文字が無い。制御文字の
// 有無だけで受理すると、符号化されていない文字列が CJK の文字列へ置き換わる。
func TestDecodeEncodedCommandLineLeavesUnencodedLexemesAlone(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		commandLine string
	}{
		{
			name:        "word in the base64 alphabet",
			commandLine: "-c Get-AppxPackage " + wordInTheBase64Alphabet,
		},
		{
			name:        "hexadecimal that looks like a hash",
			commandLine: "-u user01 -H " + hexadecimalLikeAHash + " -c cmd.exe",
		},
		{
			name:        "both in one command line",
			commandLine: "-c " + wordInTheBase64Alphabet + " " + hexadecimalLikeAHash,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			got := core.DecodeEncodedCommandLine(testCase.commandLine)

			if got.Encoded || got.Decoded {
				t.Errorf("encoded = %t and decoded = %t, want both false",
					got.Encoded, got.Decoded)
			}
			if got.CommandLine != "" {
				t.Errorf("commandLine = %q, want the empty lexeme", got.CommandLine)
			}
		})
	}
}

// **switch が宣言した文字列には、宣言の無い文字列より広い受理条件を当てる。**
//
// switch はコマンド行自身の宣言であり、復号は PowerShell が行う復号と同じである。
// 同じ文字列が、宣言の無い位置では復号の対象にならない。
func TestDecodeEncodedCommandLineAcceptsTheSwitchArgumentMoreWidely(t *testing.T) {
	withSwitch := core.DecodeEncodedCommandLine("-EncodedCommand " + wordInTheBase64Alphabet)
	if !withSwitch.Encoded || !withSwitch.Decoded {
		t.Errorf("with the switch: encoded = %t and decoded = %t, want both true",
			withSwitch.Encoded, withSwitch.Decoded)
	}

	withoutSwitch := core.DecodeEncodedCommandLine("-c " + wordInTheBase64Alphabet)
	if withoutSwitch.Encoded || withoutSwitch.Decoded {
		t.Errorf("without the switch: encoded = %t and decoded = %t, want both false",
			withoutSwitch.Encoded, withoutSwitch.Decoded)
	}
}

// switch があって続く文字列を復号できない行は、符号化を宣言していて復号できない状態を返す。
func TestDecodeEncodedCommandLineReportsTheUndecodableSwitchArgument(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		commandLine string
	}{
		{name: "no argument", commandLine: "-EncodedCommand"},
		{name: "empty argument", commandLine: "-EncodedCommand "},
		{name: "argument outside base64", commandLine: "-EncodedCommand ****"},
		{
			name:        "argument carrying control characters",
			commandLine: "-EncodedCommand " + encodedWithControlCharacters,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			got := core.DecodeEncodedCommandLine(testCase.commandLine)

			if !got.Encoded {
				t.Errorf("encoded = false, want true")
			}
			if got.Decoded {
				t.Errorf("decoded = true, want false")
			}
			if got.CommandLine != "" {
				t.Errorf("commandLine = %q, want the empty lexeme", got.CommandLine)
			}
		})
	}
}

// 符号化された文字列を持たない行は、符号化を宣言していない状態を返す。
func TestDecodeEncodedCommandLineLeavesAPlainCommandLineAlone(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		commandLine string
	}{
		{name: "empty", commandLine: ""},
		{
			name:        "plain arguments",
			commandLine: `-Command Invoke-WebRequest -Uri http://198.51.100.7/example.bin`,
		},
		{name: "windows path", commandLine: `C:\Windows\System32\cmd.exe /c dir`},
		{
			name:        "long word in the base64 alphabet",
			commandLine: "-WindowStyle Hidden ExecutionPolicyBypassUnrestricted",
		},
		{name: "switch that is not the encoded one", commandLine: "-c $value = 1"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			got := core.DecodeEncodedCommandLine(testCase.commandLine)

			if got.Encoded || got.Decoded {
				t.Errorf("encoded = %t and decoded = %t, want both false",
					got.Encoded, got.Decoded)
			}
			if got.CommandLine != "" {
				t.Errorf("commandLine = %q, want the empty lexeme", got.CommandLine)
			}
		})
	}
}

// 復号した文字列が ASCII の印字可能文字だけから成る。
func TestDecodeEncodedCommandLineReturnsPlainText(t *testing.T) {
	want := "-enc " + decodedWriteOutput
	got := core.DecodeEncodedCommandLine("-enc " + encodedWriteOutput)

	if got.CommandLine != want {
		t.Fatalf("commandLine = %q, want %q", got.CommandLine, want)
	}
	for _, r := range got.CommandLine {
		if r < ' ' || r > '~' {
			t.Errorf("the decoded line carries the character %q outside printable ASCII", r)
		}
	}
}
