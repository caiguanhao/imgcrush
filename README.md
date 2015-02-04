imgcrush
========

Recursively, losslessly and quickly compress JPG, PNG, GIF images.

Requirements:

* [mozjpeg](https://github.com/mozilla/mozjpeg)
* [pngcrush](http://pmt.sourceforge.net/pngcrush/)
* [gifsicle](http://www.lcdf.org/gifsicle/)

Usage:

```
Recursively, losslessly and quickly compress JPG, PNG, GIF images.

Usage: imgcrush [-c 2] [-o done] [DIRECTORY] ...
  -c <num>     concurrency from 1 to 8, defaults to 2
  -o <dir>     output directory, defaults to done
  --old-style  print results using old style

If no input directory provided, it will use current directory.
Images in output directory will not be used as input images.
```

Install:

If you have installed `go` already, run:

```
go get -u -v github.com/caiguanhao/imgcrush
```

Or run `imgcrush` in a Docker container:

```
# build the image first:
docker build -t imgcrush .

# run imgcrush in your current directory
docker run --rm -v="$(pwd):/imgcrush" imgcrush -c 4 .
```

References:

* [Comparison of JPEG Lossless Compression Tools](
http://blarg.co.uk/blog/comparison-of-jpeg-lossless-compression-tools)
* [ImageOptim](https://github.com/pornel/ImageOptim)
* [bounded.go](http://blog.golang.org/pipelines/bounded.go)
