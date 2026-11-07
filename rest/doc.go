// Package rest is the opt-in "rest" std module (lmcode_std_rest_v1): a REST server over standard streams.
//
// A program declares routes with rest_route, binds them to annotated "implements RestHandler" entries and calls
// rest_serve(); after the body returns, Controller.Run reads framed HTTP/1.1 requests from the input stream,
// runs the bound entry for each and writes exactly one response to the output stream. There is no network
// listener. The module is not part of the default registry: call Register to opt in.
//
// All strings are lmcode byte strings; nothing here decodes or re-encodes them.
// See spec/std-rest.md in the lmcode repository.
package rest
