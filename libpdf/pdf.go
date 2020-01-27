package libpdf

import (
	"encoding/json"
	"encoding/hex"
	"fmt"
	"C"

	"github.com/mattetti/filebuffer"
	pdf "github.com/zean00/pdfcpulite"
)

//Annotation model
type Annotation struct {
	Box     Box
	Content string
	Page    int
}

//Box type
type Box struct {
	Rect   pdf.Rectangle
	Width  float64
	Height float64
	Ratio  float64
}

//func main() {}

//export pdf_echo
func pdf_echo(input *C.char) *C.char {
	return input
}

//export pdf_get_annotation
func pdf_get_annotation(input *C.char) *C.char {
	hexstr := C.GoString(input)
	b, err := hex.DecodeString(hexstr)
	if err != nil {
		return C.CString("")
	}
	r := GetAnnotation(b)
	return C.CString(r)
}

//GetAnnotation func
func GetAnnotation(input []byte) string {
	annots, err := extractAnnotation(input)
	if err != nil {
		return ""
	}

	ba, err := json.Marshal(annots)
	if err != nil {
		return ""
	}
	return string(ba)
}

func newBox(rect pdf.Rectangle) Box {
	return Box{
		Rect:   rect,
		Width:  rect.Width(),
		Height: rect.Height(),
		Ratio:  rect.AspectRatio(),
	}
}

func extractAnnotation(bin []byte) ([]Annotation, error) {
	fb := filebuffer.New(bin)
	conf := pdf.NewDefaultConfiguration()
	ctx, err := pdf.Read(fb, conf)
	if err != nil {
		return nil, err
	}

	ctx.XRefTable.EnsurePageCount()
	//fmt.Println(len(ctx.XRefTable.Table))
	//fmt.Println(ctx.XRefTable.PageCount)
	annots := make([]Annotation, 0)

	for i := 1; i <= ctx.XRefTable.PageCount; i++ {
		pd, _, err := ctx.XRefTable.PageDict(i)
		if err != nil {
			fmt.Println(err)
			continue
		}
		v, ok := pd.Find("Annots")
		if !ok {
			continue
		}
		an := (v.(pdf.IndirectRef)).ObjectNumber
		//fmt.Println(an.Value())
		e, ok := ctx.XRefTable.Find(an.Value())
		if !ok {
			//fmt.Println("Not found")
			continue
		}

		ar, ok := e.Object.(pdf.Array)
		if !ok {
			//fmt.Println("Not an array")
			continue
		}

		if len(ar) > 0 {
			for _, v := range ar {
				obj, ok := v.(pdf.IndirectRef)
				if !ok {
					//fmt.Println("Not indirect ref")
					continue
				}
				//fmt.Println(obj.ObjectNumber)
				an, err := ctx.XRefTable.FindObject(obj.ObjectNumber.Value())
				if err != nil {
					//fmt.Println(err)
					continue
				}
				dict, ok := an.(pdf.Dict)
				if !ok {
					//fmt.Println("Not a dictionary")
					continue
				}
				//fmt.Println(dict)

				if *dict.Subtype() != "Square" {
					//fmt.Println(*dict.Subtype())
					continue
				}

				//fmt.Println(*dict.Subtype())

				r, ok := dict.Find("Rect")
				if !ok {
					fmt.Println("Rectangle not found")
					continue
				}

				var box *pdf.Rectangle
				switch v := r.(type) {
				case pdf.IndirectRef:
					rect, err := ctx.XRefTable.FindObject(v.ObjectNumber.Value())
					if err != nil {
						fmt.Println(err)
						continue
					}

					coord, ok := rect.(pdf.Array)
					if !ok {
						fmt.Println("Not an array of coordinate")
						continue
					}

					if len(coord) != 4 {
						fmt.Println("Invalid coordinate array")
						continue
					}
					box = pdf.RectForArray(coord)
				case pdf.Array:
					if len(v) != 4 {
						fmt.Println("Invalid coordinate array")
						continue
					}
					box = pdf.RectForArray(v)
				default:
					continue
				}

				//fmt.Println(box)
				content := ""
				vc, ok := dict.Find("Contents")
				if ok {
					switch h := vc.(type) {
					case pdf.HexLiteral:
						bs, err := h.Bytes()
						if err != nil {
							fmt.Println("Error get bytes")
							continue
						}
						content = string(bs)
					case pdf.StringLiteral:
						content = h.Value()
					}
				}

				if content == "" {
					vs, ok := dict.Find("Subj")
					//fmt.Println(vs)
					if ok {
						switch h := vs.(type) {
						case pdf.HexLiteral:
							bs, err := h.Bytes()
							if err != nil {
								fmt.Println("Error get bytes")
								continue
							}
							content = string(bs)
						case pdf.StringLiteral:
							content = h.Value()
						}
					}
				}

				//fmt.Println(content)

				annots = append(annots, Annotation{
					Page:    i,
					Box:     newBox(*box),
					Content: content,
				})
			}
		}

		//fmt.Println(ar)
	}

	return annots, nil
}
