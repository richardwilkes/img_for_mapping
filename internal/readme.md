The vp8 and webp directories are clones of the same directories found at golang.org/x/image.

I've modified one line of code in vp8/decode.go to allow the partition size to be up to 31 bits in size, instead of 24.
This was done to allow larger webp files to be loaded.
