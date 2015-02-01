imgcrush
========

Recursively, losslessly and quickly compress JPG, PNG, GIF files.

Uses:

* [mozjpeg](https://github.com/mozilla/mozjpeg)
* [pngcrush](http://pmt.sourceforge.net/pngcrush/)
* [gifsicle](http://www.lcdf.org/gifsicle/)

Usage:

```
Recursively, losslessly and quickly compress JPG, PNG, GIF files.

Usage: imgcrush [-c 2] [-o done] [DIRECTORY] ...
  -c <num>  concurrency from 1 to 8, defaults to 2
  -o <dir>  output directory, defaults to done

If no input directory provided, it will use current directory.
Images in output directory will not be used as input images.
```

References:

* [Comparison of JPEG Lossless Compression Tools](
http://blarg.co.uk/blog/comparison-of-jpeg-lossless-compression-tools)
* [ImageOptim](https://github.com/pornel/ImageOptim)
* [bounded.go](http://blog.golang.org/pipelines/bounded.go)
