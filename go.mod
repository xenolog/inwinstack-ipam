module github.com/inwinstack/ipam

go 1.13

require (
	github.com/inwinstack/blended v0.7.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.4.0
	github.com/thoas/go-funk v0.4.0
	k8s.io/apiextensions-apiserver v0.0.0-20190620085554-14e95df34f1f
	k8s.io/apimachinery v0.0.0-20190612205821-1799e75a0719
	k8s.io/client-go v8.0.0+incompatible
	k8s.io/klog v1.0.0
)

replace (
	github.com/inwinstack/blended => gerrit.mcp.mirantis.com/kaas-bm/inwinstack-blended v0.0.0-20191211130857-c647d33cf5cd
	k8s.io/api => k8s.io/api v0.0.0-20190620084959-7cf5895f2711
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.0.0-20190620085554-14e95df34f1f
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20190612205821-1799e75a0719
	k8s.io/client-go => k8s.io/client-go v0.0.0-20190620085101-78d2af792bab
)
