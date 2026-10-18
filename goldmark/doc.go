// Package goldmark turns a parsed goldmark tree into an nttypeset Document.
//
// It does not parse: the caller owns goldmark's parser, extensions and any
// normalization, and passes the parsed root and its source to Build. The walk
// understands the GFM extensions (strikethrough, task lists, tables) and
// goldmark-emoji; images are supplied by the caller through Options, because
// the adapter reads no files.
//
// This is the only package of the module that imports goldmark.
package goldmark
