// docs/ holds the GitHub Pages site and the README's GIFs. This go.mod makes
// it a separate module, so Go leaves it out of cuddio's module zip and
// go install never downloads the images. There is no Go code here.
module github.com/phoenix1331/cuddio/docs

go 1.26
