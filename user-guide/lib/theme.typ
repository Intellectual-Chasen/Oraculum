// 版面と、本文から呼ぶ部品を定める。

#let body-fonts = ("Noto Sans JP",)
#let code-fonts = ("DejaVu Sans Mono", "Noto Sans JP")

#let theme(title: "", body) = {
  set document(title: title)
  set page(
    paper: "a4",
    margin: (x: 22mm, y: 24mm),
    header: context {
      if counter(page).get().first() > 2 {
        set text(size: 8pt, fill: luma(110))
        title
        h(1fr)
        counter(page).display()
      }
    },
  )
  set text(font: body-fonts, lang: "ja", size: 10pt)
  set par(justify: true, leading: 0.8em, spacing: 1.2em)
  set heading(numbering: "1.1")
  show heading.where(level: 1): it => {
    pagebreak(weak: true)
    set text(size: 18pt)
    block(below: 1.2em, it)
  }
  show heading.where(level: 2): set text(size: 13pt)
  show raw: set text(font: code-fonts, size: 8.5pt)
  show raw.where(block: true): it => block(
    width: 100%,
    fill: luma(245),
    inset: 8pt,
    radius: 3pt,
    it,
  )
  set table(stroke: 0.5pt + luma(180), inset: 6pt)
  show table.cell.where(y: 0): set text(weight: "bold")
  body
}

// 画面に出る文字列を、本文と文字の色で区別して書く。
#let ui(body) = text(fill: rgb("#1d4f91"), body)

// console に入力する行と、console に出る行を並べる。
#let console(input, output: none) = {
  raw(block: true, lang: "sh", input)
  if output != none {
    text(size: 8.5pt, fill: luma(90))[出力:]
    raw(block: true, output)
  }
}

// スクリーンショットに赤枠を重ねる。marks の各要素は (x, y, w, h) で、画像の幅と高さに対する割合で書く。
// label を持つ要素は、枠の左上に番号を付ける。
#let shot(path, width: 100%, marks: (), caption: none) = {
  let frame = layout(region => {
    let w-total = if type(width) == ratio { region.width * width } else { width }
    let img = image(path, width: w-total)
    let size = measure(img)
    box(width: size.width, height: size.height, stroke: 0.5pt + luma(180), {
      img
      for m in marks {
        let (x, y, w, h) = (m.at(0), m.at(1), m.at(2), m.at(3))
        place(top + left, dx: x * size.width, dy: y * size.height,
          rect(width: w * size.width, height: h * size.height, stroke: 2pt + red))
        if m.len() > 4 {
          place(top + left, dx: x * size.width - 7pt, dy: y * size.height - 7pt,
            circle(radius: 7pt, fill: red,
              align(center + horizon, text(fill: white, size: 8pt, weight: "bold", m.at(4)))))
        }
      }
    })
  })
  figure(frame, caption: caption, kind: image, supplement: [図])
}

// 手順の前提・結果など、本文と分けて読ませる短い囲み。
#let note(label, body) = block(
  width: 100%,
  fill: rgb("#eef4fb"),
  stroke: (left: 3pt + rgb("#3b6ea8")),
  inset: 8pt,
  [*#label* #h(0.5em) #body],
)
