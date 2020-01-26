/*
Copyright 2018 The pdfcpu Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package pdflite

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/zean00/pdfcpulite/filter"
	"github.com/zean00/pdfcpulite/font"
)

const stampWithBBox = false

const (
	degToRad = math.Pi / 180
	radToDeg = 180 / math.Pi
)

// Watermark mode
const (
	WMText int = iota
	WMImage
	WMPDF
)

// Rotation along one of 2 diagonals
const (
	NoDiagonal = iota
	DiagonalLLToUR
	DiagonalULToLR
)

// Render mode
const (
	RMFill = iota
	RMStroke
	RMFillAndStroke
)

var (
	errNoContent   = errors.New("pdfcpu: stamp: PDF page has no content")
	errNoWatermark = fmt.Errorf("pdfcpu: no watermarks found - nothing removed")
)

type watermarkParamMap map[string]func(string, *Watermark) error

// Handle applies parameter completion and if successful
// parses the parameter values into import.
func (m watermarkParamMap) Handle(paramPrefix, paramValueStr string, imp *Watermark) error {

	var param string

	// Completion support
	for k := range m {
		if !strings.HasPrefix(k, paramPrefix) {
			continue
		}
		if len(param) > 0 {
			return fmt.Errorf("pdfcpu: ambiguous parameter prefix \"%s\"", paramPrefix)
		}
		param = k
	}

	if param == "" {
		return fmt.Errorf("pdfcpu: unknown parameter prefix \"%s\"", paramPrefix)
	}

	return m[param](paramValueStr, imp)
}

var wmParamMap = watermarkParamMap{
	"fontname":    parseFontName,
	"points":      parseFontSize,
	"color":       parseColor,
	"rotation":    parseRotation,
	"diagonal":    parseDiagonal,
	"opacity":     parseOpacity,
	"mode":        parseRenderMode,
	"rendermode":  parseRenderMode,
	"position":    parsePositionAnchorWM,
	"offset":      parsePositionOffsetWM,
	"scalefactor": parseScaleFactorWM,
}

// WinAnsiGlyphMap is a glyph lookup table for CP1252 character codes.
// See Annex D.2 Latin Character Set and Encodings.
var WinAnsiGlyphMap = map[int]string{
	32:  "space",          // U+0020 ' '
	33:  "exclam",         // U+0021 '!'
	34:  "quotedbl",       // U+0022 '"'
	35:  "numbersign",     // U+0023 '#'
	36:  "dollar",         // U+0024 '$'
	37:  "percent",        // U+0025 '%'
	38:  "ampersand",      // U+0026 '&'
	39:  "quotesingle",    // U+0027 '''
	40:  "parenleft",      // U+0028 '('
	41:  "parenright",     // U+0029 ')'
	42:  "asterisk",       // U+002A '*'
	43:  "plus",           // U+002B '+'
	44:  "comma",          // U+002C ','
	45:  "hyphen",         // U+002D '-'
	46:  "period",         // U+002E '.'
	47:  "slash",          // U+002F '/'
	48:  "zero",           // U+0030 '0'
	49:  "one",            // U+0031 '1'
	50:  "two",            // U+0032 '2'
	51:  "three",          // U+0033 '3'
	52:  "four",           // U+0034 '4'
	53:  "five",           // U+0035 '5'
	54:  "six",            // U+0036 '6'
	55:  "seven",          // U+0037 '7'
	56:  "eight",          // U+0038 '8'
	57:  "nine",           // U+0039 '9'
	58:  "colon",          // U+003A ':'
	59:  "semicolon",      // U+003B ';'
	60:  "less",           // U+003C '<'
	61:  "equal",          // U+003D '='
	62:  "greater",        // U+003E '>'
	63:  "question",       // U+003F '?'
	64:  "at",             // U+0040 '@'
	65:  "A",              // U+0041 'A'
	66:  "B",              // U+0042 'B'
	67:  "C",              // U+0043 'C'
	68:  "D",              // U+0044 'D'
	69:  "E",              // U+0045 'E'
	70:  "F",              // U+0046 'F'
	71:  "G",              // U+0047 'G'
	72:  "H",              // U+0048 'H'
	73:  "I",              // U+0049 'I'
	74:  "J",              // U+004A 'J'
	75:  "K",              // U+004B 'K'
	76:  "L",              // U+004C 'L'
	77:  "M",              // U+004D 'M'
	78:  "N",              // U+004E 'N'
	79:  "O",              // U+004F 'O'
	80:  "P",              // U+0050 'P'
	81:  "Q",              // U+0051 'Q'
	82:  "R",              // U+0052 'R'
	83:  "S",              // U+0053 'S'
	84:  "T",              // U+0054 'T'
	85:  "U",              // U+0055 'U'
	86:  "V",              // U+0056 'V'
	87:  "W",              // U+0057 'W'
	88:  "X",              // U+0058 'X'
	89:  "Y",              // U+0059 'Y'
	90:  "Z",              // U+005A 'Z'
	91:  "bracketleft",    // U+005B '['
	92:  "backslash",      // U+005C '\'
	93:  "bracketright",   // U+005D ']'
	94:  "asciicircum",    // U+005E '^'
	95:  "underscore",     // U+005F '_'
	96:  "grave",          // U+0060 '`'
	97:  "a",              // U+0061 'a'
	98:  "b",              // U+0062 'b'
	99:  "c",              // U+0063 'c'
	100: "d",              // U+0064 'd'
	101: "e",              // U+0065 'e'
	102: "f",              // U+0066 'f'
	103: "g",              // U+0067 'g'
	104: "h",              // U+0068 'h'
	105: "i",              // U+0069 'i'
	106: "j",              // U+006A 'j'
	107: "k",              // U+006B 'k'
	108: "l",              // U+006C 'l'
	109: "m",              // U+006D 'm'
	110: "n",              // U+006E 'n'
	111: "o",              // U+006F 'o'
	112: "p",              // U+0070 'p'
	113: "q",              // U+0071 'q'
	114: "r",              // U+0072 'r'
	115: "s",              // U+0073 's'
	116: "t",              // U+0074 't'
	117: "u",              // U+0075 'u'
	118: "v",              // U+0076 'v'
	119: "w",              // U+0077 'w'
	120: "x",              // U+0078 'x'
	121: "y",              // U+0079 'y'
	122: "z",              // U+007A 'z'
	123: "braceleft",      // U+007B '{'
	124: "bar",            // U+007C '|'
	125: "braceright",     // U+007D '}'
	126: "asciitilde",     // U+007E '~'
	128: "Euro",           // U+0080
	130: "quotesinglbase", // U+0082
	131: "florin",         // U+0083
	132: "quotedblbase",   // U+0084
	133: "ellipsis",       // U+0085
	134: "dagger",         // U+0086
	135: "daggerdbl",      // U+0087
	136: "circumflex",     // U+0088
	137: "perthousand",    // U+0089
	138: "Scaron",         // U+008A
	139: "guilsinglleft",  // U+008B
	140: "OE",             // U+008C
	142: "Zcaron",         // U+008E
	145: "quoteleft",      // U+0091
	146: "quoteright",     // U+0092
	147: "quotedblleft",   // U+0093
	148: "quotedblright",  // U+0094
	149: "bullet",         // U+0095
	150: "endash",         // U+0096
	151: "emdash",         // U+0097
	152: "tilde",          // U+0098
	153: "trademark",      // U+0099
	154: "scaron",         // U+009A
	155: "guilsinglright", // U+009B
	156: "oe",             // U+009C
	158: "zcaron",         // U+009E
	159: "Ydieresis",      // U+009F
	161: "exclamdown",     // U+00A1 '¡'
	162: "cent",           // U+00A2 '¢'
	163: "sterling",       // U+00A3 '£'
	164: "currency",       // U+00A4 '¤'
	165: "yen",            // U+00A5 '¥'
	166: "brokenbar",      // U+00A6 '¦'
	167: "section",        // U+00A7 '§'
	168: "dieresis",       // U+00A8 '¨'
	169: "copyright",      // U+00A9 '©'
	170: "ordfeminine",    // U+00AA 'ª'
	171: "guillemotleft",  // U+00AB '«'
	172: "logicalnot",     // U+00AC '¬'
	174: "registered",     // U+00AE '®'
	175: "macron",         // U+00AF '¯'
	176: "degree",         // U+00B0 '°'
	177: "plusminus",      // U+00B1 '±'
	178: "twosuperior",    // U+00B2 '²'
	179: "threesuperior",  // U+00B3 '³'
	180: "acute",          // U+00B4 '´'
	181: "mu",             // U+00B5 'µ'
	182: "paragraph",      // U+00B6 '¶'
	183: "periodcentered", // U+00B7 '·'
	184: "cedilla",        // U+00B8 '¸'
	185: "onesuperior",    // U+00B9 '¹'
	186: "ordmasculine",   // U+00BA 'º'
	187: "guillemotright", // U+00BB '»'
	188: "onequarter",     // U+00BC '¼'
	189: "onehalf",        // U+00BD '½'
	190: "threequarters",  // U+00BE '¾'
	191: "questiondown",   // U+00BF '¿'
	192: "Agrave",         // U+00C0 'À'
	193: "Aacute",         // U+00C1 'Á'
	194: "Acircumflex",    // U+00C2 'Â'
	195: "Atilde",         // U+00C3 'Ã'
	196: "Adieresis",      // U+00C4 'Ä'
	197: "Aring",          // U+00C5 'Å'
	198: "AE",             // U+00C6 'Æ'
	199: "Ccedilla",       // U+00C7 'Ç'
	200: "Egrave",         // U+00C8 'È'
	201: "Eacute",         // U+00C9 'É'
	202: "Ecircumflex",    // U+00CA 'Ê'
	203: "Edieresis",      // U+00CB 'Ë'
	204: "Igrave",         // U+00CC 'Ì'
	205: "Iacute",         // U+00CD 'Í'
	206: "Icircumflex",    // U+00CE 'Î'
	207: "Idieresis",      // U+00CF 'Ï'
	208: "Eth",            // U+00D0 'Ð'
	209: "Ntilde",         // U+00D1 'Ñ'
	210: "Ograve",         // U+00D2 'Ò'
	211: "Oacute",         // U+00D3 'Ó'
	212: "Ocircumflex",    // U+00D4 'Ô'
	213: "Otilde",         // U+00D5 'Õ'
	214: "Odieresis",      // U+00D6 'Ö'
	215: "multiply",       // U+00D7 '×'
	216: "Oslash",         // U+00D8 'Ø'
	217: "Ugrave",         // U+00D9 'Ù'
	218: "Uacute",         // U+00DA 'Ú'
	219: "Ucircumflex",    // U+00DB 'Û'
	220: "Udieresis",      // U+00DC 'Ü'
	221: "Yacute",         // U+00DD 'Ý'
	222: "Thorn",          // U+00DE 'Þ'
	223: "germandbls",     // U+00DF 'ß'
	224: "agrave",         // U+00E0 'à'
	225: "aacute",         // U+00E1 'á'
	226: "acircumflex",    // U+00E2 'â'
	227: "atilde",         // U+00E3 'ã'
	228: "adieresis",      // U+00E4 'ä'
	229: "aring",          // U+00E5 'å'
	230: "ae",             // U+00E6 'æ'
	231: "ccedilla",       // U+00E7 'ç'
	232: "egrave",         // U+00E8 'è'
	233: "eacute",         // U+00E9 'é'
	234: "ecircumflex",    // U+00EA 'ê'
	235: "edieresis",      // U+00EB 'ë'
	236: "igrave",         // U+00EC 'ì'
	237: "iacute",         // U+00ED 'í'
	238: "icircumflex",    // U+00EE 'î'
	239: "idieresis",      // U+00EF 'ï'
	240: "eth",            // U+00F0 'ð'
	241: "ntilde",         // U+00F1 'ñ'
	242: "ograve",         // U+00F2 'ò'
	243: "oacute",         // U+00F3 'ó'
	244: "ocircumflex",    // U+00F4 'ô'
	245: "otilde",         // U+00F5 'õ'
	246: "odieresis",      // U+00F6 'ö'
	247: "divide",         // U+00F7 '÷'
	248: "oslash",         // U+00F8 'ø'
	249: "ugrave",         // U+00F9 'ù'
	250: "uacute",         // U+00FA 'ú'
	251: "ucircumflex",    // U+00FB 'û'
	252: "udieresis",      // U+00FC 'ü'
	253: "yacute",         // U+00FD 'ý'
	254: "thorn",          // U+00FE 'þ'
	255: "ydieresis",      // U+00FF 'ÿ'
}

type matrix [3][3]float64

var identMatrix = matrix{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}

func (m matrix) multiply(n matrix) matrix {
	var p matrix
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			for k := 0; k < 3; k++ {
				p[i][j] += m[i][k] * n[k][j]
			}
		}
	}
	return p
}

func (m matrix) String() string {
	return fmt.Sprintf("%3.2f %3.2f %3.2f\n%3.2f %3.2f %3.2f\n%3.2f %3.2f %3.2f\n",
		m[0][0], m[0][1], m[0][2],
		m[1][0], m[1][1], m[1][2],
		m[2][0], m[2][1], m[2][2])
}

// SimpleColor is a simple rgb wrapper.
type SimpleColor struct {
	r, g, b float32 // intensities between 0 and 1.
}

func (sc SimpleColor) String() string {
	return fmt.Sprintf("r=%1.1f g=%1.1f b=%1.1f", sc.r, sc.g, sc.b)
}

type formCache map[Rectangle]*IndirectRef

type pdfResources struct {
	content []byte
	resDict *IndirectRef
	width   int
	height  int
}

// Watermark represents the basic structure and command details for the commands "Stamp" and "Watermark".
type Watermark struct {

	// configuration
	Mode              int         // WMText, WMImage or WMPDF
	TextString        string      // raw display text.
	TextLines         []string    // display multiple lines of text.
	FileName          string      // display pdf page or png image.
	Page              int         // the page number of a PDF file. 0 means multistamp/multiwatermark.
	OnTop             bool        // if true this is a STAMP else this is a WATERMARK.
	Pos               anchor      // position anchor, one of tl,tc,tr,l,c,r,bl,bc,br.
	Dx, Dy            int         // anchor offset.
	FontName          string      // supported are Adobe base fonts only. (as of now: Helvetica, Times-Roman, Courier)
	FontSize          int         // font scaling factor.
	ScaledFontSize    int         // font scaling factor for a specific page
	Color             SimpleColor // fill color(=non stroking color).
	Rotation          float64     // rotation to apply in degrees. -180 <= x <= 180
	Diagonal          int         // paint along the diagonal.
	UserRotOrDiagonal bool        // true if one of rotation or diagonal provided overriding the default.
	Opacity           float64     // opacity the displayed text. 0 <= x <= 1
	RenderMode        int         // fill=0, stroke=1 fill&stroke=2
	Scale             float64     // relative scale factor. 0 <= x <= 1
	ScaleAbs          bool        // true for absolute scaling.
	Update            bool        // true for updating instead of adding a page watermark.

	// resources
	ocg, extGState, font, image *IndirectRef

	// for an image or PDF watermark
	width, height int // image or page dimensions.

	// for a PDF watermark
	pdfRes map[int]pdfResources

	// page specific
	bb      *Rectangle   // bounding box of the form representing this watermark.
	vp      *Rectangle   // page dimensions for text alignment.
	pageRot float64      // page rotation in effect.
	form    *IndirectRef // Forms are dependent on given page dimensions.

	// house keeping
	objs   IntSet    // objects for which wm has been applied already.
	fCache formCache // form cache.
}

// DefaultWatermarkConfig returns the default configuration.
func DefaultWatermarkConfig() *Watermark {
	return &Watermark{
		Page:       0,
		FontName:   "Helvetica",
		FontSize:   24,
		Pos:        Center,
		Scale:      0.5,
		ScaleAbs:   false,
		Color:      SimpleColor{0.5, 0.5, 0.5}, // gray
		Diagonal:   DiagonalLLToUR,
		Opacity:    1.0,
		RenderMode: RMFill,
		pdfRes:     map[int]pdfResources{},
		objs:       IntSet{},
		fCache:     formCache{},
		TextLines:  []string{},
	}
}

func (wm Watermark) typ() string {
	if wm.isImage() {
		return "image"
	}
	if wm.isPDF() {
		return "pdf"
	}
	return "text"
}

func (wm Watermark) String() string {

	var s string
	if !wm.OnTop {
		s = "not "
	}

	t := wm.TextString
	if len(t) == 0 {
		t = wm.FileName
	}

	sc := "relative"
	if wm.ScaleAbs {
		sc = "absolute"
	}

	bbox := ""
	if wm.bb != nil {
		bbox = (*wm.bb).String()
	}

	vp := ""
	if wm.vp != nil {
		vp = (*wm.vp).String()
	}

	return fmt.Sprintf("Watermark: <%s> is %son top, typ:%s\n"+
		"%s %d points\n"+
		"PDFpage#: %d\n"+
		"scaling: %.1f %s\n"+
		"color: %s\n"+
		"rotation: %.1f\n"+
		"diagonal: %d\n"+
		"opacity: %.1f\n"+
		"renderMode: %d\n"+
		"bbox:%s\n"+
		"vp:%s\n"+
		"pageRotation: %.1f\n",
		t, s, wm.typ(),
		wm.FontName, wm.FontSize,
		wm.Page,
		wm.Scale, sc,
		wm.Color,
		wm.Rotation,
		wm.Diagonal,
		wm.Opacity,
		wm.RenderMode,
		bbox,
		vp,
		wm.pageRot,
	)
}

// OnTopString returns "watermark" or "stamp" whichever applies.
func (wm Watermark) OnTopString() string {
	s := "watermark"
	if wm.OnTop {
		s = "stamp"
	}
	return s
}

func (wm Watermark) multiStamp() bool {
	return wm.Page == 0
}

func (wm Watermark) calcMaxTextWidth() float64 {

	var maxWidth float64

	for _, l := range wm.TextLines {
		w := font.TextWidth(l, wm.FontName, wm.ScaledFontSize)
		if w > maxWidth {
			maxWidth = w
		}
	}

	return maxWidth
}

func parsePositionAnchorWM(s string, wm *Watermark) error {
	// Note: Reliable with non rotated pages only!
	switch s {
	case "tl":
		wm.Pos = TopLeft
	case "tc":
		wm.Pos = TopCenter
	case "tr":
		wm.Pos = TopRight
	case "l":
		wm.Pos = Left
	case "c":
		wm.Pos = Center
	case "r":
		wm.Pos = Right
	case "bl":
		wm.Pos = BottomLeft
	case "bc":
		wm.Pos = BottomCenter
	case "br":
		wm.Pos = BottomRight
	default:
		return fmt.Errorf("pdfcpu: unknown position anchor: %s", s)
	}

	return nil
}

func parsePositionOffsetWM(s string, wm *Watermark) error {

	var err error

	d := strings.Split(s, " ")
	if len(d) != 2 {
		return fmt.Errorf("pdfcpu: illegal position offset string: need 2 numeric values, %s\n", s)
	}

	wm.Dx, err = strconv.Atoi(d[0])
	if err != nil {
		return err
	}

	wm.Dy, err = strconv.Atoi(d[1])
	if err != nil {
		return err
	}

	return nil
}

func parseScaleFactorWM(s string, wm *Watermark) (err error) {
	wm.Scale, wm.ScaleAbs, err = parseScaleFactor(s)
	return err
}

func parseFontName(s string, wm *Watermark) error {
	if !font.SupportedFont(s) {
		return fmt.Errorf("pdfcpu: %s is unsupported, please refer to \"pdfcpu fonts list\".\n", s)
	}
	wm.FontName = s
	return nil
}

func parseFontSize(s string, wm *Watermark) error {

	fs, err := strconv.Atoi(s)
	if err != nil {
		return err
	}

	wm.FontSize = fs

	return nil
}

func parseScaleFactor(s string) (float64, bool, error) {

	ss := strings.Split(s, " ")
	if len(ss) > 2 {
		return 0, false, fmt.Errorf("illegal scale string: 0.0 <= i <= 1.0 {abs|rel}, %s\n", s)
	}

	sc, err := strconv.ParseFloat(ss[0], 64)
	if err != nil {
		return 0, false, fmt.Errorf("scale factor must be a float value: %s\n", ss[0])
	}
	if sc < 0 || sc > 1 {
		return 0, false, fmt.Errorf("illegal scale factor: 0.0 <= s <= 1.0, %s\n", ss[0])
	}

	var scaleAbs bool

	if len(ss) == 2 {
		switch ss[1] {
		case "a", "abs":
			scaleAbs = true

		case "r", "rel":
			scaleAbs = false

		default:
			return 0, false, fmt.Errorf("illegal scale mode: abs|rel, %s\n", ss[1])
		}
	}

	return sc, scaleAbs, nil
}

func parseColor(s string, wm *Watermark) error {

	cs := strings.Split(s, " ")
	if len(cs) != 3 {
		return fmt.Errorf("pdfcpu: illegal color string: 3 intensities 0.0 <= i <= 1.0, %s\n", s)
	}

	r, err := strconv.ParseFloat(cs[0], 32)
	if err != nil {
		return fmt.Errorf("red must be a float value: %s\n", cs[0])
	}
	if r < 0 || r > 1 {
		return errors.New("pdfcpu: red: a color value is an intensity between 0.0 and 1.0")
	}
	wm.Color.r = float32(r)

	g, err := strconv.ParseFloat(cs[1], 32)
	if err != nil {
		return fmt.Errorf("pdfcpu: green must be a float value: %s\n", cs[1])
	}
	if g < 0 || g > 1 {
		return errors.New("pdfcpu: green: a color value is an intensity between 0.0 and 1.0")
	}
	wm.Color.g = float32(g)

	b, err := strconv.ParseFloat(cs[2], 32)
	if err != nil {
		return fmt.Errorf("pdfcpu: blue must be a float value: %s\n", cs[2])
	}
	if b < 0 || b > 1 {
		return errors.New("pdfcpu: blue: a color value is an intensity between 0.0 and 1.0")
	}
	wm.Color.b = float32(b)

	return nil
}

func parseRotation(s string, wm *Watermark) error {

	if wm.UserRotOrDiagonal {
		return errors.New("pdfcpu: please specify rotation or diagonal (r or d)")
	}

	r, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("pdfcpu: rotation must be a float value: %s\n", s)
	}
	if r < -180 || r > 180 {
		return fmt.Errorf("pdfcpu: illegal rotation: -180 <= r <= 180 degrees, %s\n", s)
	}

	wm.Rotation = r
	wm.Diagonal = NoDiagonal
	wm.UserRotOrDiagonal = true

	return nil
}

func parseDiagonal(s string, wm *Watermark) error {

	if wm.UserRotOrDiagonal {
		return errors.New("pdfcpu: please specify rotation or diagonal (r or d)")
	}

	d, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("pdfcpu: illegal diagonal value: allowed 1 or 2, %s\n", s)
	}
	if d != DiagonalLLToUR && d != DiagonalULToLR {
		return errors.New("pdfcpu: diagonal: 1..lower left to upper right, 2..upper left to lower right")
	}

	wm.Diagonal = d
	wm.Rotation = 0
	wm.UserRotOrDiagonal = true

	return nil
}

func parseOpacity(s string, wm *Watermark) error {

	o, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("pdfcpu: opacity must be a float value: %s\n", s)
	}
	if o < 0 || o > 1 {
		return fmt.Errorf("pdfcpu: illegal opacity: 0.0 <= r <= 1.0, %s\n", s)
	}
	wm.Opacity = o

	return nil
}

func parseRenderMode(s string, wm *Watermark) error {

	m, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("pdfcpu: illegal render mode value: allowed 0,1,2, %s\n", s)
	}
	if m != RMFill && m != RMStroke && m != RMFillAndStroke {
		return errors.New("pdfcpu: valid rendermodes: 0..fill, 1..stroke, 2..fill&stroke")
	}
	wm.RenderMode = m

	return nil
}

func parseWatermarkDetails(mode int, modeParm, s string, onTop bool) (*Watermark, error) {

	wm := DefaultWatermarkConfig()
	wm.OnTop = onTop
	setWatermarkType(mode, modeParm, wm)

	ss := strings.Split(s, ",")
	if len(ss) < 1 || len(ss[0]) == 0 {
		return wm, nil
	}

	for _, s := range ss {

		ss1 := strings.Split(s, ":")
		if len(ss1) != 2 {
			return nil, parseWatermarkError(onTop)
		}

		paramPrefix := strings.TrimSpace(ss1[0])
		paramValueStr := strings.TrimSpace(ss1[1])

		if err := wmParamMap.Handle(paramPrefix, paramValueStr, wm); err != nil {
			return nil, err
		}
	}

	return wm, nil
}

// ParseTextWatermarkDetails parses a text Watermark/Stamp command string into an internal structure.
func ParseTextWatermarkDetails(text, desc string, onTop bool) (*Watermark, error) {
	return parseWatermarkDetails(WMText, text, desc, onTop)
}

// ParseImageWatermarkDetails parses a text Watermark/Stamp command string into an internal structure.
func ParseImageWatermarkDetails(fileName, desc string, onTop bool) (*Watermark, error) {
	return parseWatermarkDetails(WMImage, fileName, desc, onTop)
}

// ParsePDFWatermarkDetails parses a text Watermark/Stamp command string into an internal structure.
func ParsePDFWatermarkDetails(fileName, desc string, onTop bool) (*Watermark, error) {
	return parseWatermarkDetails(WMPDF, fileName, desc, onTop)
}

func (wm Watermark) calcMinFontSize(w float64) int {

	var minSize int

	for _, l := range wm.TextLines {
		w := font.Size(l, wm.FontName, w)
		if minSize == 0.0 {
			minSize = w
		}
		if w < minSize {
			minSize = w
		}
	}

	return minSize
}

// IsPDF returns whether the watermark content is an image or text.
func (wm Watermark) isPDF() bool {
	return len(wm.FileName) > 0 && strings.ToLower(filepath.Ext(wm.FileName)) == ".pdf"
}

// IsImage returns whether the watermark content is an image or text.
func (wm Watermark) isImage() bool {
	return len(wm.FileName) > 0 && strings.ToLower(filepath.Ext(wm.FileName)) != ".pdf"
}

func (wm *Watermark) calcBoundingBox(pageNr int) {

	var bb *Rectangle

	if wm.isImage() || wm.isPDF() {

		if wm.isPDF() {
			wm.width = wm.pdfRes[wm.Page].width
			wm.height = wm.pdfRes[wm.Page].height
			if wm.multiStamp() {
				i := pageNr
				if i > len(wm.pdfRes) {
					i = len(wm.pdfRes)
				}
				wm.width = wm.pdfRes[i].width
				wm.height = wm.pdfRes[i].height
			}
		}

		bb = RectForDim(float64(wm.width), float64(wm.height))
		ar := bb.AspectRatio()

		if wm.ScaleAbs {
			bb.UR.X = wm.Scale * bb.Width()
			bb.UR.Y = bb.UR.X / ar

			wm.bb = bb
			return
		}

		if ar >= 1 {
			bb.UR.X = wm.Scale * wm.vp.Width()
			bb.UR.Y = bb.UR.X / ar
		} else {
			bb.UR.Y = wm.Scale * wm.vp.Height()
			bb.UR.X = bb.UR.Y * ar
		}

		wm.bb = bb
		return
	}

	// Text watermark

	var w float64
	if wm.ScaleAbs {
		wm.ScaledFontSize = int(float64(wm.FontSize) * wm.Scale)
		w = wm.calcMaxTextWidth()
	} else {
		w = wm.Scale * wm.vp.Width()
		wm.ScaledFontSize = wm.calcMinFontSize(w)
	}

	fbb := font.BoundingBox(wm.FontName)
	h := (float64(wm.ScaledFontSize) * fbb.Height() / 1000)
	if len(wm.TextLines) > 1 {
		h += float64(len(wm.TextLines)-1) * float64(wm.ScaledFontSize)
	}

	wm.bb = Rect(0, 0, w, h)
	return
}

func (wm *Watermark) calcTransformMatrix() *matrix {

	var sin, cos float64
	r := wm.Rotation

	if wm.Diagonal != NoDiagonal {

		// Calculate the angle of the diagonal with respect of the aspect ratio of the bounding box.
		r = math.Atan(wm.vp.Height()/wm.vp.Width()) * float64(radToDeg)

		if wm.bb.AspectRatio() < 1 {
			r -= 90
		}

		if wm.Diagonal == DiagonalULToLR {
			r = -r
		}

	}

	sin = math.Sin(float64(r) * float64(degToRad))
	cos = math.Cos(float64(r) * float64(degToRad))

	// 1) Rotate
	m1 := identMatrix
	m1[0][0] = cos
	m1[0][1] = sin
	m1[1][0] = -sin
	m1[1][1] = cos

	// 2) Translate
	m2 := identMatrix

	var dy float64
	if !wm.isImage() && !wm.isPDF() {
		dy = wm.bb.LL.Y
	}

	ll := lowerLeftCorner(wm.vp.Width(), wm.vp.Height(), wm.bb.Width(), wm.bb.Height(), wm.Pos)
	m2[2][0] = ll.X + wm.bb.Width()/2 + float64(wm.Dx) + sin*(wm.bb.Height()/2+dy) - cos*wm.bb.Width()/2
	m2[2][1] = ll.Y + wm.bb.Height()/2 + float64(wm.Dy) - cos*(wm.bb.Height()/2+dy) - sin*wm.bb.Width()/2

	m := m1.multiply(m2)
	return &m
}

func onTopString(onTop bool) string {
	e := "watermark"
	if onTop {
		e = "stamp"
	}
	return e
}

func parseWatermarkError(onTop bool) error {
	s := onTopString(onTop)
	return fmt.Errorf("Invalid %s configuration string. Please consult pdfcpu help %s.\n", s, s)
}

func setWatermarkType(mode int, s string, wm *Watermark) error {

	wm.Mode = mode

	switch mode {
	case WMText:
		wm.TextString = s
		bb := []byte{}
		for _, r := range s {
			bb = append(bb, byte(r))
		}
		s = strings.ReplaceAll(string(bb), "\\n", "\n")
		for _, l := range strings.FieldsFunc(s, func(c rune) bool { return c == 0x0a }) {
			wm.TextLines = append(wm.TextLines, l)
		}

	case WMImage:
		ext := strings.ToLower(filepath.Ext(s))
		if !MemberOf(ext, []string{".jpg", ".jpeg", ".png", ".tif", ".tiff"}) {
			return errors.New("imageFileName has to have one of these extensions: jpg, jpeg, png, tif, tiff")
		}
		wm.FileName = s

	case WMPDF:
		ss := strings.Split(s, ":")
		ext := strings.ToLower(filepath.Ext(ss[0]))
		if !MemberOf(ext, []string{".pdf"}) {
			return fmt.Errorf("%s is not a PDF file", ss[0])
		}
		wm.FileName = ss[0]
		if len(ss) > 1 {
			// Parse page number for PDF watermarks.
			var err error
			wm.Page, err = strconv.Atoi(ss[1])
			if err != nil {
				return fmt.Errorf("illegal PDF page number: %s\n", ss[1])
			}
		}

	}

	return nil
}

func coreFontDict(fontName string) Dict {
	d := NewDict()
	d.InsertName("Type", "Font")
	d.InsertName("Subtype", "Type1")
	d.InsertName("BaseFont", fontName)

	if fontName != "Symbol" && fontName != "ZapfDingbats" {
		encDict := Dict(
			map[string]Object{
				"Type":         Name("Encoding"),
				"BaseEncoding": Name("WinAnsiEncoding"),
				"Differences":  Array{Integer(172), Name("Euro")},
			},
		)
		d.Insert("Encoding", encDict)
	}
	return d
}

func ttfWidths(xRefTable *XRefTable, ttf font.TTFLight) (*IndirectRef, error) {

	// we have tff firstchar, lastchar !

	missingW := ttf.GlyphWidths[0]

	w := make([]int, 256)

	for i := 0; i < 256; i++ {
		if i < 32 || WinAnsiGlyphMap[i] == ".notdef" {
			w[i] = missingW
			continue
		}

		pos, ok := ttf.Chars[uint16(i)]
		if !ok {
			//fmt.Printf("Character %s missing\n", metrics.WinAnsiGlyphMap[i])
			w[i] = missingW
			continue
		}

		w[i] = ttf.GlyphWidths[pos]
	}

	a := make(Array, 256-32)
	for i := 32; i < 256; i++ {
		a[i-32] = Integer(w[i])
	}

	return xRefTable.IndRefForNewObject(a)
}

func ttfFontDescriptorFlags(ttf font.TTFLight) uint32 {

	// Bits:
	// 1 FixedPitch
	// 2 Serif
	// 3 Symbolic
	// 4 Script/cursive
	// 6 Nonsymbolic
	// 7 Italic
	// 17 AllCap

	flags := uint32(0)

	// Bit 1
	//fmt.Printf("fixedPitch: %t\n", ttf.FixedPitch)
	if ttf.FixedPitch {
		flags |= 0x01
	}

	// Bit 6 Set for non symbolic
	// Note: Symbolic fonts are unsupported.
	flags |= 0x20

	// Bit 7
	//fmt.Printf("italicAngle: %f\n", ttf.ItalicAngle)
	if ttf.ItalicAngle != 0 {
		flags |= 0x40
	}

	//fmt.Printf("flags: %08x\n", flags)

	return flags
}

/*
func ttfFontFile(xRefTable *XRefTable, ttf font.TTFLight, fontName string) (*IndirectRef, error) {

	sd := &StreamDict{Dict: NewDict()}
	sd.InsertName("Filter", filter.Flate)
	sd.FilterPipeline = []PDFFilter{{Name: filter.Flate, DecodeParms: nil}}

	bb, err := font.Read(fontName)
	if err != nil {
		return nil, err
	}
	//fmt.Printf("read %d fontfile bytes\n", len(bb))
	sd.InsertInt("Length1", len(bb))

	sd.Content = bb

	if err := encodeStream(sd); err != nil {
		return nil, err
	}

	return xRefTable.IndRefForNewObject(*sd)
}

func ttfFontDescriptor(xRefTable *XRefTable, ttf font.TTFLight, fontName string) (*IndirectRef, error) {

	fontFile, err := ttfFontFile(xRefTable, ttf, fontName)
	if err != nil {
		return nil, err
	}

	d := Dict(
		map[string]Object{
			"Type":        Name("FontDescriptor"),
			"FontName":    Name(fontName),
			"Flags":       Integer(ttfFontDescriptorFlags(ttf)),
			"FontBBox":    NewNumberArray(ttf.LLx, ttf.LLy, ttf.URx, ttf.URy),
			"ItalicAngle": Float(ttf.ItalicAngle),
			"Ascent":      Integer(ttf.Ascent),
			"Descent":     Integer(ttf.Descent),
			//"Leading": // The spacing between baselines of consecutive lines of text.
			"CapHeight": Integer(ttf.CapHeight),
			"StemV":     Integer(70), // Irrelevant for embedded files.
			"FontFile2": *fontFile,
		},
	)

	return xRefTable.IndRefForNewObject(d)
}

func userFontDict(xRefTable *XRefTable, fontName string) (Dict, error) {

	ttf := font.UserFontMetrics[fontName]

	d := NewDict()
	d.InsertName("Type", "Font")
	d.InsertName("Subtype", "TrueType")
	d.InsertName("BaseFont", fontName)
	d.InsertInt("FirstChar", 32)
	d.InsertInt("LastChar", 255)

	w, err := ttfWidths(xRefTable, ttf)
	if err != nil {
		return nil, err
	}
	d.Insert("Widths", *w)

	fd, err := ttfFontDescriptor(xRefTable, ttf, fontName)
	if err != nil {
		return nil, err
	}
	d.Insert("FontDescriptor", *fd)

	d.InsertName("Encoding", "WinAnsiEncoding")

	return d, nil
}

func createFontResForWM(xRefTable *XRefTable, wm *Watermark) error {

	var (
		d   Dict
		err error
	)

	if font.IsCoreFont(wm.FontName) {
		d = coreFontDict(wm.FontName)
	} else {
		d, err = userFontDict(xRefTable, wm.FontName)
		if err != nil {
			return err
		}
	}

	ir, err := xRefTable.IndRefForNewObject(d)
	if err != nil {
		return err
	}

	wm.font = ir

	return nil
}
*/
func contentStream(xRefTable *XRefTable, o Object) ([]byte, error) {

	o, err := xRefTable.Dereference(o)
	if err != nil {
		return nil, err
	}

	bb := []byte{}

	switch o := o.(type) {

	case StreamDict:

		// Decode streamDict for supported filters only.
		err := decodeStream(&o)
		if err == filter.ErrUnsupportedFilter {
			return nil, errors.New("pdfcpu: unsupported filter: unable to decode content for PDF watermark")
		}
		if err != nil {
			return nil, err
		}

		bb = o.Content

	case Array:
		for _, o := range o {

			if o == nil {
				continue
			}

			sd, err := xRefTable.DereferenceStreamDict(o)
			if err != nil {
				return nil, err
			}

			// Decode streamDict for supported filters only.
			err = decodeStream(sd)
			if err == filter.ErrUnsupportedFilter {
				return nil, errors.New("pdfcpu: unsupported filter: unable to decode content for PDF watermark")
			}
			if err != nil {
				return nil, err
			}

			bb = append(bb, sd.Content...)
		}
	}

	if len(bb) == 0 {
		return nil, errNoContent
	}

	return bb, nil
}

func identifyObjNrs(ctx *Context, o Object, migrated map[int]int, objNrs IntSet) error {

	switch o := o.(type) {

	case IndirectRef:
		objNr := o.ObjectNumber.Value()
		if migrated[objNr] > 0 {
			return nil
		}
		if objNr >= *ctx.Size {
			//fmt.Printf("%d > %d(ctx.Size)\n", objNr, *ctx.Size)
			return nil
		}
		objNrs[objNr] = true

		o1, err := ctx.Dereference(o)
		if err != nil {
			return err
		}

		if err = identifyObjNrs(ctx, o1, migrated, objNrs); err != nil {
			return err
		}

	case Dict:
		for _, v := range o {
			if err := identifyObjNrs(ctx, v, migrated, objNrs); err != nil {
				return err
			}
		}

	case StreamDict:
		for _, v := range o.Dict {
			if err := identifyObjNrs(ctx, v, migrated, objNrs); err != nil {
				return err
			}
		}

	case Array:
		for _, v := range o {
			if err := identifyObjNrs(ctx, v, migrated, objNrs); err != nil {
				return err
			}
		}

	}

	return nil
}

// migrateObject migrates o from ctxSource into ctxDest.
func migrateObject(ctxSource, ctxDest *Context, migrated map[int]int, o Object) (Object, error) {

	// Collect referenced objNrs of o in ctxSource that have not been migrated.
	objNrs := IntSet{}
	if err := identifyObjNrs(ctxSource, o, migrated, objNrs); err != nil {
		return nil, err
	}

	// Create a mapping from migration candidates in ctxSource to new objs in ctxDest.
	for k := range objNrs {
		migrated[k] = *ctxDest.Size
		*ctxDest.Size++
	}

	// Patch indRefs reachable by o in ctxSource.
	if po := patchObject(o, migrated); po != nil {
		o = po
	}

	for k := range objNrs {
		patchObject(ctxSource.Table[k].Object, migrated)
		v := migrated[k]
		ctxDest.Table[v] = ctxSource.Table[k]
	}

	return o, nil
}

func createPDFRes(ctx, otherCtx *Context, pageNr int, migrated map[int]int, wm *Watermark) error {

	pdfRes := pdfResources{}
	xRefTable := ctx.XRefTable
	otherXRefTable := otherCtx.XRefTable

	// Locate page dict & resource dict of PDF stamp.
	d, inhPAttrs, err := otherXRefTable.PageDict(pageNr)
	if err != nil {
		return err
	}
	if d == nil {
		return fmt.Errorf("pdfcpu: unknown page number: %d\n", pageNr)
	}

	// Retrieve content stream bytes of page dict.
	o, found := d.Find("Contents")
	if !found {
		return errors.New("pdfcpu: PDF page has no content")
	}
	pdfRes.content, err = contentStream(otherXRefTable, o)
	if err != nil {
		return err
	}

	// Migrate external resource dict into ctx.
	if _, err = migrateObject(otherCtx, ctx, migrated, inhPAttrs.resources); err != nil {
		return err
	}

	// Create an object for resource dict in xRefTable.
	ir, err := xRefTable.IndRefForNewObject(inhPAttrs.resources)
	if err != nil {
		return err
	}

	pdfRes.resDict = ir

	vp := viewPort(xRefTable, inhPAttrs)
	pdfRes.width = int(vp.Width())
	pdfRes.height = int(vp.Height())

	wm.pdfRes[pageNr] = pdfRes

	return nil
}

/*
func createPDFResForWM(ctx *Context, wm *Watermark) error {

	// The stamp pdf is assumed to be valid.
	otherCtx, err := ReadFile(wm.FileName, NewDefaultConfiguration())
	if err != nil {
		return err
	}

	if err := otherCtx.EnsurePageCount(); err != nil {
		return nil
	}

	migrated := map[int]int{}

	if !wm.multiStamp() {
		if err := createPDFRes(ctx, otherCtx, wm.Page, migrated, wm); err != nil {
			return err
		}
	} else {
		j := otherCtx.PageCount
		if ctx.PageCount < otherCtx.PageCount {
			j = ctx.PageCount
		}
		for i := 1; i <= j; i++ {
			if err := createPDFRes(ctx, otherCtx, i, migrated, wm); err != nil {
				return err
			}
		}
	}

	return nil
}

func createImageResource(xRefTable *XRefTable, r io.Reader) (*IndirectRef, int, int, error) {

	bb, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, 0, 0, err
	}

	var sd *StreamDict
	r = bytes.NewReader(bb)

	// We identify JPG via its magic bytes.
	if bytes.HasPrefix(bb, []byte("\xff\xd8")) {
		// Process JPG by wrapping byte stream into DCTEncoded object stream.
		c, _, err := image.DecodeConfig(r)
		if err != nil {
			return nil, 0, 0, err
		}

		sd, err = ReadJPEG(xRefTable, bb, c)
		if err != nil {
			return nil, 0, 0, err
		}

	} else {
		// Process other formats by decoding into an image
		// and subsequent object stream encoding,
		img, _, err := image.Decode(r)
		if err != nil {
			return nil, 0, 0, err
		}

		sd, err = imgToImageDict(xRefTable, img)
		if err != nil {
			return nil, 0, 0, err
		}
	}

	w := *sd.IntEntry("Width")
	h := *sd.IntEntry("Height")

	indRef, err := xRefTable.IndRefForNewObject(*sd)
	if err != nil {
		return nil, 0, 0, err
	}

	return indRef, w, h, nil
}


func createImageResForWM(xRefTable *XRefTable, wm *Watermark) (err error) {

	f, err := os.Open(wm.FileName)
	if err != nil {
		return err
	}
	defer f.Close()

	wm.image, wm.width, wm.height, err = createImageResource(xRefTable, f)

	return err
}

func createResourcesForWM(ctx *Context, wm *Watermark) error {

	xRefTable := ctx.XRefTable

	if wm.isPDF() {
		return createPDFResForWM(ctx, wm)
	}

	if wm.isImage() {
		return createImageResForWM(xRefTable, wm)
	}

	return createFontResForWM(xRefTable, wm)
}
*/
func ensureOCG(xRefTable *XRefTable, wm *Watermark) error {

	name := "Background"
	subt := "BG"
	if wm.OnTop {
		name = "Watermark"
		subt = "FG"
	}

	d := Dict(
		map[string]Object{
			"Name": StringLiteral(name),
			"Type": Name("OCG"),
			"Usage": Dict(
				map[string]Object{
					"PageElement": Dict(map[string]Object{"Subtype": Name(subt)}),
					"View":        Dict(map[string]Object{"ViewState": Name("ON")}),
					"Print":       Dict(map[string]Object{"PrintState": Name("ON")}),
					"Export":      Dict(map[string]Object{"ExportState": Name("ON")}),
				},
			),
		},
	)

	ir, err := xRefTable.IndRefForNewObject(d)
	if err != nil {
		return err
	}

	wm.ocg = ir

	return nil
}

func prepareOCPropertiesInRoot(ctx *Context, wm *Watermark) error {

	rootDict, err := ctx.Catalog()
	if err != nil {
		return err
	}

	if o, ok := rootDict.Find("OCProperties"); ok {

		// Set wm.ocg indRef
		d, err := ctx.DereferenceDict(o)
		if err != nil {
			return err
		}

		o, found := d.Find("OCGs")
		if found {
			a, err := ctx.DereferenceArray(o)
			if err != nil {
				return fmt.Errorf("OCProperties: corrupt OCGs element")
			}

			ir, ok := a[0].(IndirectRef)
			if !ok {
				return fmt.Errorf("OCProperties: corrupt OCGs element")
			}
			wm.ocg = &ir

			return nil
		}
	}

	if err := ensureOCG(ctx.XRefTable, wm); err != nil {
		return err
	}

	optionalContentConfigDict := Dict(
		map[string]Object{
			"AS": Array{
				Dict(
					map[string]Object{
						"Category": NewNameArray("View"),
						"Event":    Name("View"),
						"OCGs":     Array{*wm.ocg},
					},
				),
				Dict(
					map[string]Object{
						"Category": NewNameArray("Print"),
						"Event":    Name("Print"),
						"OCGs":     Array{*wm.ocg},
					},
				),
				Dict(
					map[string]Object{
						"Category": NewNameArray("Export"),
						"Event":    Name("Export"),
						"OCGs":     Array{*wm.ocg},
					},
				),
			},
			"ON":       Array{*wm.ocg},
			"Order":    Array{},
			"RBGroups": Array{},
		},
	)

	d := Dict(
		map[string]Object{
			"OCGs": Array{*wm.ocg},
			"D":    optionalContentConfigDict,
		},
	)

	rootDict.Update("OCProperties", d)
	return nil
}

func createFormResDict(xRefTable *XRefTable, pageNr int, wm *Watermark) (*IndirectRef, error) {

	if wm.isPDF() {
		i := wm.Page
		if wm.multiStamp() {
			maxStampPageNr := len(wm.pdfRes)
			i = pageNr
			if pageNr > maxStampPageNr {
				i = maxStampPageNr
			}
		}
		return wm.pdfRes[i].resDict, nil
	}

	if wm.isImage() {

		d := Dict(
			map[string]Object{
				"ProcSet": NewNameArray("PDF", "ImageC"),
				"XObject": Dict(map[string]Object{"Im0": *wm.image}),
			},
		)

		return xRefTable.IndRefForNewObject(d)

	}

	d := Dict(
		map[string]Object{
			"Font":    Dict(map[string]Object{wm.FontName: *wm.font}),
			"ProcSet": NewNameArray("PDF", "Text"),
		},
	)

	return xRefTable.IndRefForNewObject(d)
}

func cachedForm(wm *Watermark) bool {
	return !wm.isPDF() || !wm.multiStamp()
}

func pdfFormContent(w io.Writer, pageNr int, wm *Watermark) error {
	cs := wm.pdfRes[wm.Page].content
	if wm.multiStamp() {
		maxStampPageNr := len(wm.pdfRes)
		i := pageNr
		if pageNr > maxStampPageNr {
			i = maxStampPageNr
		}
		cs = wm.pdfRes[i].content
	}
	sc := wm.Scale
	if !wm.ScaleAbs {
		sc = wm.bb.Width() / float64(wm.width)
	}
	fmt.Fprintf(w, "%f 0 0 %f 0 0 cm ", sc, sc)
	_, err := w.Write(cs)
	return err
}

func imageFormContent(w io.Writer, pageNr int, wm *Watermark) {
	fmt.Fprintf(w, "q %f 0 0 %f 0 0 cm /Im0 Do Q", wm.bb.Width(), wm.bb.Height()) // TODO dont need Q
}

func textFormContent(w io.Writer, pageNr int, wm *Watermark) {
	// 12 font points result in a vertical displacement of 9.47
	dy := -float64(wm.ScaledFontSize) / 12 * 9.47
	wmForm := "0 g 0 G 0 i 0 J []0 d 0 j 1 w 10 M 0 Tc 0 Tw 100 Tz 0 TL %d Tr 0 Ts "
	fmt.Fprintf(w, wmForm, wm.RenderMode)
	j := 1
	for i := len(wm.TextLines) - 1; i >= 0; i-- {

		sw := font.TextWidth(wm.TextLines[i], wm.FontName, wm.ScaledFontSize)
		dx := wm.bb.Width()/2 - sw/2

		fmt.Fprintf(w, "BT /%s %d Tf %f %f %f rg %f %f Td (%s) Tj ET ",
			wm.FontName, wm.ScaledFontSize, wm.Color.r, wm.Color.g, wm.Color.b, dx, dy+float64(j*wm.ScaledFontSize), wm.TextLines[i])
		j++
	}
}

func formContent(w io.Writer, pageNr int, wm *Watermark) error {
	switch true {
	case wm.isPDF():
		return pdfFormContent(w, pageNr, wm)
	case wm.isImage():
		imageFormContent(w, pageNr, wm)
	default:
		textFormContent(w, pageNr, wm)
	}
	return nil
}

/*
func createForm(xRefTable *XRefTable, pageNr int, wm *Watermark, withBB bool) error {

	// The forms bounding box is dependent on the page dimensions.
	wm.calcBoundingBox(pageNr)
	bb := wm.bb

	if cachedForm(wm) || pageNr > len(wm.pdfRes) {
		// Use cached form.
		ir, ok := wm.fCache[*bb.Rectangle]
		if ok {
			wm.form = ir
			return nil
		}
	}

	var b bytes.Buffer
	if err := formContent(&b, pageNr, wm); err != nil {
		return err
	}

	// Paint bounding box
	if withBB {
		urx := bb.UR.X
		ury := bb.UR.Y
		if wm.isPDF() {
			sc := wm.Scale
			if !wm.ScaleAbs {
				sc = bb.Width() / float64(wm.width)
			}
			urx /= sc
			ury /= sc
		}
		fmt.Fprintf(&b, "[]0 d 2 w %.0f %.0f m %.0f %.0f l %.0f %.0f l %.0f %.0f l s ",
			bb.LL.X, bb.LL.Y,
			urx, bb.LL.Y,
			urx, ury,
			bb.LL.X, ury,
		)
	}

	ir, err := createFormResDict(xRefTable, pageNr, wm)
	if err != nil {
		return err
	}

	sd := StreamDict{
		Dict: Dict(
			map[string]Object{
				"Type":      Name("XObject"),
				"Subtype":   Name("Form"),
				"BBox":      bb.Array(),
				"Matrix":    NewIntegerArray(1, 0, 0, 1, 0, 0),
				"OC":        *wm.ocg,
				"Resources": *ir,
			},
		),
		Content: b.Bytes(),
	}

	if err = encodeStream(&sd); err != nil {
		return err
	}

	ir, err = xRefTable.IndRefForNewObject(sd)
	if err != nil {
		return err
	}

	wm.form = ir

	if cachedForm(wm) || pageNr >= len(wm.pdfRes) {
		// Cache form.
		wm.fCache[*wm.bb.Rectangle] = ir
	}

	return nil
}

func createExtGStateForStamp(xRefTable *XRefTable, wm *Watermark) error {

	d := Dict(
		map[string]Object{
			"Type": Name("ExtGState"),
			"CA":   Float(wm.Opacity),
			"ca":   Float(wm.Opacity),
		},
	)

	ir, err := xRefTable.IndRefForNewObject(d)
	if err != nil {
		return err
	}

	wm.extGState = ir

	return nil
}

func insertPageResourcesForWM(xRefTable *XRefTable, pageDict Dict, wm *Watermark, gsID, xoID string) error {

	resourceDict := Dict(
		map[string]Object{
			"ExtGState": Dict(map[string]Object{gsID: *wm.extGState}),
			"XObject":   Dict(map[string]Object{xoID: *wm.form}),
		},
	)

	pageDict.Insert("Resources", resourceDict)

	return nil
}

func updatePageResourcesForWM(xRefTable *XRefTable, resDict Dict, wm *Watermark, gsID, xoID *string) error {

	o, ok := resDict.Find("ExtGState")
	if !ok {
		resDict.Insert("ExtGState", Dict(map[string]Object{*gsID: *wm.extGState}))
	} else {
		d, _ := xRefTable.DereferenceDict(o)
		for i := 0; i < 1000; i++ {
			*gsID = "GS" + strconv.Itoa(i)
			if _, found := d.Find(*gsID); !found {
				break
			}
		}
		d.Insert(*gsID, *wm.extGState)
	}

	o, ok = resDict.Find("XObject")
	if !ok {
		resDict.Insert("XObject", Dict(map[string]Object{*xoID: *wm.form}))
	} else {
		d, _ := xRefTable.DereferenceDict(o)
		for i := 0; i < 1000; i++ {
			*xoID = "Fm" + strconv.Itoa(i)
			if _, found := d.Find(*xoID); !found {
				break
			}
		}
		d.Insert(*xoID, *wm.form)
	}

	return nil
}
*/
func wmContent(wm *Watermark, gsID, xoID string) []byte {

	m := wm.calcTransformMatrix()

	insertOCG := " /Artifact <</Subtype /Watermark /Type /Pagination >>BDC q %.2f %.2f %.2f %.2f %.2f %.2f cm /%s gs /%s Do Q EMC "

	var b bytes.Buffer
	fmt.Fprintf(&b, insertOCG, m[0][0], m[0][1], m[1][0], m[1][1], m[2][0], m[2][1], gsID, xoID)

	return b.Bytes()
}

func insertPageContentsForWM(xRefTable *XRefTable, pageDict Dict, wm *Watermark, gsID, xoID string) error {

	sd := &StreamDict{Dict: NewDict()}

	sd.Content = wmContent(wm, gsID, xoID)

	err := encodeStream(sd)
	if err != nil {
		return err
	}

	ir, err := xRefTable.IndRefForNewObject(*sd)
	if err != nil {
		return err
	}

	pageDict.Insert("Contents", *ir)

	return nil
}

func updatePageContentsForWM(xRefTable *XRefTable, obj Object, wm *Watermark, gsID, xoID string) error {

	var entry *XRefTableEntry
	var objNr int

	ir, ok := obj.(IndirectRef)
	if ok {
		objNr = ir.ObjectNumber.Value()
		if wm.objs[objNr] {
			// wm already applied to this content stream.
			return nil
		}
		genNr := ir.GenerationNumber.Value()
		entry, _ = xRefTable.FindTableEntry(objNr, genNr)
		obj = entry.Object
	}

	switch o := obj.(type) {

	case StreamDict:

		err := patchContentForWM(&o, gsID, xoID, wm, true)
		if err != nil {
			return err
		}

		entry.Object = o
		wm.objs[objNr] = true

	case Array:

		// Get stream dict for first element.
		o1 := o[0]
		ir, _ := o1.(IndirectRef)
		objNr = ir.ObjectNumber.Value()
		genNr := ir.GenerationNumber.Value()
		entry, _ := xRefTable.FindTableEntry(objNr, genNr)
		sd, _ := (entry.Object).(StreamDict)

		if len(o) == 1 || !wm.OnTop {

			if wm.objs[objNr] {
				// wm already applied to this content stream.
				return nil
			}

			err := patchContentForWM(&sd, gsID, xoID, wm, true)
			if err != nil {
				return err
			}
			entry.Object = sd
			wm.objs[objNr] = true
			return nil
		}

		if wm.objs[objNr] {
			// wm already applied to this content stream.
		} else {
			// Patch first content stream.
			err := patchFirstContentForWM(&sd)
			if err != nil {
				return err
			}
			entry.Object = sd
			wm.objs[objNr] = true
		}

		// Patch last content stream.
		o1 = o[len(o)-1]

		ir, _ = o1.(IndirectRef)
		objNr = ir.ObjectNumber.Value()
		if wm.objs[objNr] {
			// wm already applied to this content stream.
			return nil
		}

		genNr = ir.GenerationNumber.Value()
		entry, _ = xRefTable.FindTableEntry(objNr, genNr)
		sd, _ = (entry.Object).(StreamDict)

		err := patchContentForWM(&sd, gsID, xoID, wm, false)
		if err != nil {
			return err
		}

		entry.Object = sd
		wm.objs[objNr] = true
	}

	return nil
}

func viewPort(xRefTable *XRefTable, a *InheritedPageAttrs) *Rectangle {

	visibleRegion := a.mediaBox
	if a.cropBox != nil {
		visibleRegion = a.cropBox
	}

	return visibleRegion
}

/*
func addPageWatermark(xRefTable *XRefTable, i int, wm *Watermark) error {

	fmt.Printf("addPageWatermark page:%d\n", i)
	if wm.Update {
		fmt.Println("Updating")
		if _, err := removePageWatermark(xRefTable, i); err != nil {
			return err
		}
	}

	d, inhPAttrs, err := xRefTable.PageDict(i)
	if err != nil {
		return err
	}

	wm.vp = viewPort(xRefTable, inhPAttrs)

	err = createForm(xRefTable, i, wm, stampWithBBox)
	if err != nil {
		return err
	}

	wm.pageRot = float64(inhPAttrs.rotate)

	fmt.Printf("\n%s\n", wm)

	gsID := "GS0"
	xoID := "Fm0"

	if inhPAttrs.resources == nil {
		err = insertPageResourcesForWM(xRefTable, d, wm, gsID, xoID)
	} else {
		err = updatePageResourcesForWM(xRefTable, inhPAttrs.resources, wm, &gsID, &xoID)
	}
	if err != nil {
		return err
	}

	obj, found := d.Find("Contents")
	if !found {
		return insertPageContentsForWM(xRefTable, d, wm, gsID, xoID)
	}

	return updatePageContentsForWM(xRefTable, obj, wm, gsID, xoID)
}
*/
func patchContentForWM(sd *StreamDict, gsID, xoID string, wm *Watermark, saveGState bool) error {

	// Decode streamDict for supported filters only.
	err := decodeStream(sd)
	if err == filter.ErrUnsupportedFilter {
		fmt.Println("unsupported filter: unable to patch content with watermark.")
		return nil
	}
	if err != nil {
		return err
	}

	bb := wmContent(wm, gsID, xoID)

	if wm.OnTop {
		if saveGState {
			sd.Content = append([]byte("q "), sd.Content...)
		}
		sd.Content = append(sd.Content, []byte(" Q")...)
		sd.Content = append(sd.Content, bb...)
	} else {
		sd.Content = append(bb, sd.Content...)
	}

	return encodeStream(sd)
}

func patchFirstContentForWM(sd *StreamDict) error {

	err := decodeStream(sd)
	if err == filter.ErrUnsupportedFilter {
		fmt.Println("unsupported filter: unable to patch content with watermark.")
		return nil
	}
	if err != nil {
		return err
	}

	sd.Content = append([]byte("q "), sd.Content...)

	return encodeStream(sd)
}

/*
// AddWatermarks adds watermarks to all pages selected.
func AddWatermarks(ctx *Context, selectedPages IntSet, wm *Watermark) error {

	fmt.Printf("AddWatermarks wm:\n%s\n", wm)

	xRefTable := ctx.XRefTable

	if err := prepareOCPropertiesInRoot(ctx, wm); err != nil {
		return err
	}

	if err := createResourcesForWM(ctx, wm); err != nil {
		return err
	}

	if err := createExtGStateForStamp(xRefTable, wm); err != nil {
		return err
	}

	if selectedPages == nil || len(selectedPages) == 0 {
		selectedPages = IntSet{}
		for i := 1; i <= ctx.PageCount; i++ {
			selectedPages[i] = true
		}
	}

	for k, v := range selectedPages {
		if v {
			if err := addPageWatermark(ctx.XRefTable, k, wm); err != nil {
				return err
			}
		}
	}

	xRefTable.EnsureVersionForWriting()

	return nil
}
*/
func removeResDictEntry(xRefTable *XRefTable, d *Dict, entry string, ids []string, i int) error {

	o, ok := d.Find(entry)
	if !ok {
		return fmt.Errorf("page %d: corrupt resource dict", i)
	}

	d1, err := xRefTable.DereferenceDict(o)
	if err != nil {
		return err
	}

	for _, id := range ids {
		o, ok := d1.Find(id)
		if ok {
			err = xRefTable.deleteObject(o)
			if err != nil {
				return err
			}
			d1.Delete(id)
		}
	}

	if d1.Len() == 0 {
		d.Delete(entry)
	}

	return nil
}

func removeExtGStates(xRefTable *XRefTable, d *Dict, ids []string, i int) error {
	return removeResDictEntry(xRefTable, d, "ExtGState", ids, i)
}

func removeForms(xRefTable *XRefTable, d *Dict, ids []string, i int) error {
	return removeResDictEntry(xRefTable, d, "XObject", ids, i)
}

func removeArtifacts(sd *StreamDict, i int) (ok bool, extGStates []string, forms []string, err error) {

	err = decodeStream(sd)
	if err == filter.ErrUnsupportedFilter {
		fmt.Printf("unsupported filter: unable to patch content with watermark for page %d\n", i)
		return false, nil, nil, nil
	}
	if err != nil {
		return false, nil, nil, err
	}

	var patched bool

	// Watermarks may be at the beginning or the end of the content stream.

	for {
		s := string(sd.Content)
		beg := strings.Index(s, "/Artifact <</Subtype /Watermark /Type /Pagination >>BDC")
		if beg < 0 {
			break
		}

		end := strings.Index(s[beg:], "EMC")
		if end < 0 {
			break
		}

		// Check for usage of resources.
		t := s[beg : beg+end]

		i := strings.Index(t, "/GS")
		if i > 0 {
			j := i + 3
			k := strings.Index(t[j:], " gs")
			if k > 0 {
				extGStates = append(extGStates, "GS"+t[j:j+k])
			}
		}

		i = strings.Index(t, "/Fm")
		if i > 0 {
			j := i + 3
			k := strings.Index(t[j:], " Do")
			if k > 0 {
				forms = append(forms, "Fm"+t[j:j+k])
			}
		}

		// TODO Remove whitespace until 0x0a
		sd.Content = append(sd.Content[:beg], sd.Content[beg+end+3:]...)
		patched = true
	}

	if patched {
		err = encodeStream(sd)
	}

	return patched, extGStates, forms, err
}

func removeArtifactsFromPage(xRefTable *XRefTable, sd *StreamDict, resDict *Dict, i int) (bool, error) {

	// Remove watermark artifacts and locate id's
	// of used extGStates and forms.
	ok, extGStates, forms, err := removeArtifacts(sd, i)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}

	// Remove obsolete extGStates from page resource dict.
	err = removeExtGStates(xRefTable, resDict, extGStates, i)
	if err != nil {
		return false, err
	}

	// Remove obsolete extGStatesforms from page resource dict.
	return true, removeForms(xRefTable, resDict, forms, i)
}

func locatePageContentAndResourceDict(xRefTable *XRefTable, i int) (Object, Dict, error) {

	d, _, err := xRefTable.PageDict(i)
	if err != nil {
		return nil, nil, err
	}

	o, found := d.Find("Resources")
	if !found {
		return nil, nil, fmt.Errorf("page %d: no resource dict found\n", i)
	}

	resDict, err := xRefTable.DereferenceDict(o)
	if err != nil {
		return nil, nil, err
	}

	o, found = d.Find("Contents")
	if !found {
		return nil, nil, fmt.Errorf("page %d: no page watermark found", i)
	}

	return o, resDict, nil
}

func removePageWatermark(xRefTable *XRefTable, i int) (bool, error) {

	o, resDict, err := locatePageContentAndResourceDict(xRefTable, i)
	if err != nil {
		return false, err
	}

	found := false
	var entry *XRefTableEntry

	ir, ok := o.(IndirectRef)
	if ok {
		objNr := ir.ObjectNumber.Value()
		genNr := ir.GenerationNumber.Value()
		entry, _ = xRefTable.FindTableEntry(objNr, genNr)
		o = entry.Object
	}

	switch o := o.(type) {

	case StreamDict:
		ok, err := removeArtifactsFromPage(xRefTable, &o, &resDict, i)
		if err != nil {
			return false, err
		}
		if !found && ok {
			found = true
		}
		entry.Object = o

	case Array:
		// Get stream dict for first element.
		o1 := o[0]
		ir, _ := o1.(IndirectRef)
		objNr := ir.ObjectNumber.Value()
		genNr := ir.GenerationNumber.Value()
		entry, _ := xRefTable.FindTableEntry(objNr, genNr)
		sd, _ := (entry.Object).(StreamDict)

		ok, err := removeArtifactsFromPage(xRefTable, &sd, &resDict, i)
		if err != nil {
			return false, err
		}
		if !found && ok {
			found = true
			entry.Object = sd
		}

		if len(o) > 1 {
			// Get stream dict for last element.
			o1 := o[len(o)-1]
			ir, _ := o1.(IndirectRef)
			objNr = ir.ObjectNumber.Value()
			genNr := ir.GenerationNumber.Value()
			entry, _ := xRefTable.FindTableEntry(objNr, genNr)
			sd, _ := (entry.Object).(StreamDict)

			ok, err = removeArtifactsFromPage(xRefTable, &sd, &resDict, i)
			if err != nil {
				return false, err
			}
			if !found && ok {
				found = true
				entry.Object = sd
			}
		}

	}

	/*
		Supposedly the form needs a PieceInfo in order to be recognized by Acrobat like so:

			<PieceInfo, <<
				<ADBE_CompoundType, <<
					<DocSettings, (61 0 R)>
					<LastModified, (D:20190830152436+02'00')>
					<Private, Watermark>
				>>>
			>>>

	*/

	return found, nil
}

func locateOCGs(ctx *Context) (Array, error) {

	rootDict, err := ctx.Catalog()
	if err != nil {
		return nil, err
	}

	o, ok := rootDict.Find("OCProperties")
	if !ok {
		return nil, errNoWatermark
	}

	d, err := ctx.DereferenceDict(o)
	if err != nil {
		return nil, err
	}

	o, found := d.Find("OCGs")
	if !found {
		return nil, errNoWatermark
	}

	return ctx.DereferenceArray(o)
}

// RemoveWatermarks removes watermarks for all pages selected.
func RemoveWatermarks(ctx *Context, selectedPages IntSet) error {

	fmt.Printf("RemoveWatermarks\n")

	a, err := locateOCGs(ctx)
	if err != nil {
		return err
	}

	found := false

	for _, o := range a {
		d, err := ctx.DereferenceDict(o)
		if err != nil {
			return err
		}

		if o == nil {
			continue
		}

		if *d.Type() != "OCG" {
			continue
		}

		n := d.StringEntry("Name")
		if n == nil {
			continue
		}

		if *n != "Background" && *n != "Watermark" {
			continue
		}

		found = true
		break
	}

	if !found {
		return errNoWatermark
	}

	var removedSmth bool

	for k, v := range selectedPages {
		if !v {
			continue
		}

		ok, err := removePageWatermark(ctx.XRefTable, k)
		if err != nil {
			return err
		}

		if ok {
			removedSmth = true
		}
	}

	if !removedSmth {
		return errNoWatermark
	}

	return nil
}

func detectArtifacts(xRefTable *XRefTable, sd *StreamDict) (bool, error) {

	if err := decodeStream(sd); err != nil {
		return false, err
	}

	// Watermarks may be at the beginning or at the end of the content stream.
	i := strings.Index(string(sd.Content), "/Artifact <</Subtype /Watermark /Type /Pagination >>BDC")
	return i >= 0, nil
}

func findPageWatermarks(xRefTable *XRefTable, pageDictIndRef *IndirectRef) (bool, error) {

	d, err := xRefTable.DereferenceDict(*pageDictIndRef)
	if err != nil {
		return false, err
	}

	o, found := d.Find("Contents")
	if !found {
		return false, errors.New("missing page contents")
	}

	var entry *XRefTableEntry

	ir, ok := o.(IndirectRef)
	if ok {
		objNr := ir.ObjectNumber.Value()
		genNr := ir.GenerationNumber.Value()
		entry, _ = xRefTable.FindTableEntry(objNr, genNr)
		o = entry.Object
	}

	switch o := o.(type) {

	case StreamDict:
		return detectArtifacts(xRefTable, &o)

	case Array:
		// Get stream dict for first element.
		o1 := o[0]
		ir, _ := o1.(IndirectRef)
		objNr := ir.ObjectNumber.Value()
		genNr := ir.GenerationNumber.Value()
		entry, _ := xRefTable.FindTableEntry(objNr, genNr)
		sd, _ := (entry.Object).(StreamDict)

		ok, err := detectArtifacts(xRefTable, &sd)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}

		if len(o) > 1 {
			// Get stream dict for last element.
			o1 := o[len(o)-1]
			ir, _ := o1.(IndirectRef)
			objNr = ir.ObjectNumber.Value()
			genNr := ir.GenerationNumber.Value()
			entry, _ := xRefTable.FindTableEntry(objNr, genNr)
			sd, _ := (entry.Object).(StreamDict)
			return detectArtifacts(xRefTable, &sd)
		}

	}

	return false, nil
}

// DetectWatermarks checks ctx for watermarks
// and records the result to xRefTable.Watermarked.
func DetectWatermarks(ctx *Context) error {

	a, err := locateOCGs(ctx)
	if err != nil {
		if err == errNoWatermark {
			ctx.Watermarked = false
			return nil
		}
		return err
	}

	found := false

	for _, o := range a {
		d, err := ctx.DereferenceDict(o)
		if err != nil {
			return err
		}

		if o == nil {
			continue
		}

		if *d.Type() != "OCG" {
			continue
		}

		n := d.StringEntry("Name")
		if n == nil {
			continue
		}

		if *n != "Background" && *n != "Watermark" {
			continue
		}

		found = true
		break
	}

	if !found {
		ctx.Watermarked = false
		return nil
	}

	return ctx.DetectPageTreeWatermarks()
}
