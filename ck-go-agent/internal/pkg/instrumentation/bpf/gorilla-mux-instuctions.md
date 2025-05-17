I want to have an instrumentation for gorilla mux library.

I am particularly instrested in having a uprobe for the method.

func (r *Route) Match(req *http.Request, match *RouteMatch) bool 

This is inside the gorilla/mux/route.go file.

The module is
module github.com/gorilla/mux

The function should print the respone that is received in RouteMatch. 
The Uprobe needs to be attached to the entry & exit of the function.

The uprobe should be written in the same instrumetation/bpf folder as where this file is present.
You can look at other uprobes present in the folder for reference.
This package is a fork from opentelemetry-go-instrumentation. However, we have made some changes to the code.

Before you start, create a plan of what you want to do first.
Ask me for any questions. Dont assume anything.