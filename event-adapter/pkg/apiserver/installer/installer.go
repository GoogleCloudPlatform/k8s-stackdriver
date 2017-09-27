/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package installer

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	gpath "path"
	"reflect"
	"strings"
	"time"

	restful "github.com/emicklei/go-restful"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apiserver/pkg/endpoints"
	"k8s.io/apiserver/pkg/endpoints/handlers"
	"k8s.io/apiserver/pkg/endpoints/handlers/negotiation"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"

	specificcontext "github.com/GoogleCloudPlatform/k8s-stackdriver/event-adapter/pkg/apiserver/installer/context"
)

// NB: the contents of this file should mostly be a subset of the functionality
// in "k8s.io/apiserver/pkg/endpoints".  It would be nice to eventual figure out
// a way to not have to recreate/copy a bunch of the structure from the normal API
// installer, so that this trivially tracks changes to the main installer.

// EventsAPIGroupVersion is similar to "k8s.io/apiserver/pkg/endpoints".APIGroupVersion,
// except that it installs the metrics REST handlers, which use wildcard resources
// and subresources.
//
// This basically only serves the limitted use case required by the metrics API server --
// the only verb accepted is GET (and perhaps WATCH in the future).
type EventsAPIGroupVersion struct {
	DynamicStorage rest.Storage

	*endpoints.APIGroupVersion
}

// InstallREST registers the dynamic REST handlers into a restful Container.
// It is expected that the provided path root prefix will serve all operations.  Root MUST
// NOT end in a slash.  It should mirror InstallREST in the plain APIGroupVersion.
func (g *EventsAPIGroupVersion) InstallREST(container *restful.Container) error {
	installer := g.newDynamicInstaller()
	ws := installer.NewWebService()

	registrationErrors := installer.Install(ws)
	lister := g.ResourceLister
	if lister == nil {
		return fmt.Errorf("must provide a dynamic lister for dynamic API groups")
	}
	endpoints.AddSupportedResourcesWebService(g.Serializer, ws, g.GroupVersion, lister)
	container.Add(ws)
	return utilerrors.NewAggregate(registrationErrors)
}

// newDynamicInstaller is a helper to create the installer.  It mirrors
// newInstaller in APIGroupVersion.
func (g *EventsAPIGroupVersion) newDynamicInstaller() *EventsAPIInstaller {
	prefix := gpath.Join(g.Root, g.GroupVersion.Group, g.GroupVersion.Version)
	installer := &EventsAPIInstaller{
		group:             g,
		prefix:            prefix,
		minRequestTimeout: g.MinRequestTimeout,
	}

	return installer
}

// EventsAPIInstaller is a specialized API installer for the metrics API.
// It is intended to be fully compliant with the Kubernetes API server conventions,
// but serves wildcard resource/subresource routes instead of hard-coded resources
// and subresources.
type EventsAPIInstaller struct {
	group             *EventsAPIGroupVersion
	prefix            string // Path prefix where API resources are to be registered.
	minRequestTimeout time.Duration

	// TODO: do we want to embed a normal API installer here so we can serve normal
	// endpoints side by side with dynamic ones (from the same API group)?
}

// Install installs handlers for API resources.
func (a *EventsAPIInstaller) Install(ws *restful.WebService) (errors []error) {
	errors = make([]error, 0)

	err := a.registerResourceHandlers(a.group.DynamicStorage, ws)
	if err != nil {
		errors = append(errors, fmt.Errorf("error in registering the resource: %v", err))
	}

	return errors
}

// NewWebService creates a new restful webservice with the api installer's prefix and version.
func (a *EventsAPIInstaller) NewWebService() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(a.prefix)
	// a.prefix contains "prefix/group/version"
	ws.Doc("API at " + a.prefix)
	// Backwards compatibility, we accepted objects with empty content-type at V1.
	// If we stop using go-restful, we can default empty content-type to application/json on an
	// endpoint by endpoint basis
	ws.Consumes("*/*")
	mediaTypes, streamMediaTypes := negotiation.MediaTypesForSerializer(a.group.Serializer)
	ws.Produces(append(mediaTypes, streamMediaTypes...)...)
	ws.ApiVersion(a.group.GroupVersion.String())

	return ws
}

// registerResourceHandlers registers the resource handlers for events.
// Compared to the normal installer, this plays fast and loose a bit, but should still
// follow the API conventions.
func (a *EventsAPIInstaller) registerResourceHandlers(storage rest.Storage, ws *restful.WebService) error {
	context := a.group.Context

	optionsExternalVersion := a.group.GroupVersion
	if a.group.OptionsExternalVersion != nil {
		optionsExternalVersion = *a.group.OptionsExternalVersion
	}

	mapping, err := a.restMapping()
	if err != nil {
		return err
	}

	fqKindToRegister, err := a.getResourceKind(storage)
	if err != nil {
		return err
	}

	kind := fqKindToRegister.Kind

	lister := storage.(rest.Lister)
	list := lister.NewList()
	listGVKs, _, err := a.group.Typer.ObjectKinds(list)
	if err != nil {
		return err
	}
	versionedListPtr, err := a.group.Creater.New(a.group.GroupVersion.WithKind(listGVKs[0].Kind))
	if err != nil {
		return err
	}
	versionedList := indirectArbitraryPointer(versionedListPtr)

	versionedListOptions, err := a.group.Creater.New(optionsExternalVersion.WithKind("ListOptions"))
	if err != nil {
		return err
	}

	scope := mapping.Scope
	nameParam := ws.PathParameter("name", "name of the described event").DataType("string")
	namespaceParam := ws.PathParameter(scope.ArgumentName(), scope.ParamDescription()).DataType("string")

	namespacedParams := []*restful.Parameter{
		nameParam,
		namespaceParam,
	}

	// REGISTER: BY NAME & NAMESPACE
	namespacedPath := scope.ParamName() + "/{" + scope.ArgumentName() + "}/events/{name}"
	mediaTypes, streamMediaTypes := negotiation.MediaTypesForSerializer(a.group.Serializer)
	allMediaTypes := append(mediaTypes, streamMediaTypes...)
	ws.Produces(allMediaTypes...)
	reqScope := handlers.RequestScope{
		Serializer:      a.group.Serializer,
		ParameterCodec:  a.group.ParameterCodec,
		Creater:         a.group.Creater,
		Convertor:       a.group.Convertor,
		Copier:          a.group.Copier,
		Typer:           a.group.Typer,
		UnsafeConvertor: a.group.UnsafeConvertor,
		// TODO: This seems wrong for cross-group subresources. It makes an assumption that a subresource and its parent are in the same group version. Revisit this.
		Resource:         a.group.GroupVersion.WithResource("*"),
		Subresource:      "*",
		Kind:             fqKindToRegister,
		MetaGroupVersion: metav1.SchemeGroupVersion,
	}
	itemPathPrefix := gpath.Join(a.prefix, scope.ParamName()) + "/"
	itemPathMiddle := "/events/"
	itemPathFn := func(name, namespace string) bytes.Buffer {
		var buf bytes.Buffer
		buf.WriteString(itemPathPrefix)
		buf.WriteString(url.QueryEscape(name))
		buf.WriteString(itemPathMiddle)
		buf.WriteString(url.QueryEscape(namespace))
		return buf
	}
	ctxFn := func(req *restful.Request) request.Context {
		var ctx request.Context
		if ctx != nil {
			if existingCtx, ok := context.Get(req.Request); ok {
				ctx = existingCtx
			}
		}
		if ctx == nil {
			ctx = request.NewContext()
		}
		ctx = request.WithUserAgent(ctx, req.HeaderParameter("User-Agent"))
		name := req.PathParameter("name")
		ctx = specificcontext.WithRequestInformation(ctx, name, "GET")
		return ctx
	}
	reqScope.ContextFunc = ctxFn
	if a.group.MetaGroupVersion != nil {
		reqScope.MetaGroupVersion = *a.group.MetaGroupVersion
	}
	// install the namespace-scoped route
	reqScope.Namer = scopeNaming{scope, a.group.Linker, itemPathFn, false}
	namespacedHandler := handlers.ListResource(lister, nil, reqScope, false, a.minRequestTimeout)
	doc := "get event with the given name and namespace"
	namespacedRoute := ws.GET(namespacedPath).To(namespacedHandler).
		Doc(doc).
		Param(ws.QueryParameter("pretty", "If 'true', then the output is pretty printed.")).
		Operation("listNamespaced"+kind).
		Produces(allMediaTypes...).
		Returns(http.StatusOK, "OK", versionedList).
		Writes(versionedList)
	if err := addObjectParams(ws, namespacedRoute, versionedListOptions); err != nil {
		return err
	}
	addParams(namespacedRoute, namespacedParams)
	ws.Route(namespacedRoute)

	//PATH namespaces/{namespace}/events/{eventName}
	namespacedListPath := scope.ParamName() + "/{" + scope.ArgumentName() + "}/events"
	namespacedListParams := []*restful.Parameter{
		namespaceParam,
	}
	docList := map[string]string{
		"GET":  "list events with the given namespace",
		"POST": "create an event with the given namepsace",
	}
	itemPathListPrefix := gpath.Join(a.prefix, scope.ParamName()) + "/"
	itemPathListMiddle := "/events"
	itemPathListFn := func(name, namespace string) bytes.Buffer {
		var buf bytes.Buffer
		buf.WriteString(itemPathListPrefix)
		buf.WriteString(url.QueryEscape(namespace))
		buf.WriteString(itemPathListMiddle)
		return buf
	}
	methods := map[string]func(ws *restful.WebService, path string) *restful.RouteBuilder{
		"GET": func(ws *restful.WebService, path string) *restful.RouteBuilder {
			return ws.GET(path)
		},
		"POST": func(ws *restful.WebService, path string) *restful.RouteBuilder {
			return ws.POST(path)
		},
	}
	for k, v := range methods {
		method := k
		namespacedCtxFn := func(req *restful.Request) request.Context {
			var ctx request.Context
			if ctx != nil {
				if existingCtx, ok := context.Get(req.Request); ok {
					ctx = existingCtx
				}
			}
			if ctx == nil {
				ctx = request.NewContext()
			}

			ctx = request.WithUserAgent(ctx, req.HeaderParameter("User-Agent"))
			ctx = specificcontext.WithRequestListInformation(ctx, method)
			return ctx
		}
		reqScope.ContextFunc = namespacedCtxFn
		reqScope.Namer = scopeNaming{scope, a.group.Linker, itemPathListFn, false}
		namespacedHandler := handlers.ListResource(lister, nil, reqScope, false, a.minRequestTimeout)
		routeBuilder := v(ws, namespacedListPath)
		namespacedRoute := routeBuilder.To(namespacedHandler).
			Doc(docList[method]).
			Param(ws.QueryParameter("pretty", "If 'true', then the output is pretty printed.")).
			Produces(allMediaTypes...).
			Returns(http.StatusOK, "OK", versionedList).
			Writes(versionedList)
		if err := addObjectParams(ws, namespacedRoute, versionedListOptions); err != nil {
			return err
		}
		addParams(namespacedRoute, namespacedListParams)
		ws.Route(namespacedRoute)
	}

	//LIST ALL EVENTS
	eventsListPath := "events"
	ctxFnList := func(req *restful.Request) request.Context {
		var ctx request.Context
		if ctx != nil {
			if existingCtx, ok := context.Get(req.Request); ok {
				ctx = existingCtx
			}
		}
		if ctx == nil {
			ctx = request.NewContext()
		}
		ctx = request.WithUserAgent(ctx, req.HeaderParameter("User-Agent"))
		ctx = specificcontext.WithRequestListInformation(ctx, "GET")
		return ctx
	}
	reqScope.ContextFunc = ctxFnList
	reqScope.Namer = rootScopeNaming{scope, a.group.Linker, a.prefix, eventsListPath}
	rootScopedHandler := handlers.ListResource(lister, nil, reqScope, false, a.minRequestTimeout)
	doc = "list all the events"
	rootScopedRoute := ws.GET(eventsListPath).To(rootScopedHandler).
		Doc(doc).
		Param(ws.QueryParameter("pretty", "If 'true', then the output is pretty printed.")).
		Operation("listNamespaced"+kind).
		Produces(allMediaTypes...).
		Returns(http.StatusOK, "OK", versionedList).
		Writes(versionedList)
	if err := addObjectParams(ws, rootScopedRoute, versionedListOptions); err != nil {
		return err
	}
	ws.Route(rootScopedRoute)

	return nil
}

// This magic incantation returns *ptrToObject for an arbitrary pointer
func indirectArbitraryPointer(ptrToObject interface{}) interface{} {
	return reflect.Indirect(reflect.ValueOf(ptrToObject)).Interface()
}

// getResourceKind returns the external group version kind registered for the given storage object.
func (a *EventsAPIInstaller) getResourceKind(storage rest.Storage) (schema.GroupVersionKind, error) {
	object := storage.New()
	fqKinds, _, err := a.group.Typer.ObjectKinds(object)
	if err != nil {
		return schema.GroupVersionKind{}, err
	}

	// a given go type can have multiple potential fully qualified kinds.  Find the one that corresponds with the group
	// we're trying to register here
	fqKindToRegister := schema.GroupVersionKind{}
	for _, fqKind := range fqKinds {
		if fqKind.Group == a.group.GroupVersion.Group {
			fqKindToRegister = a.group.GroupVersion.WithKind(fqKind.Kind)
			break
		}

		// TODO: keep rid of extensions api group dependency here
		// This keeps it doing what it was doing before, but it doesn't feel right.
		if fqKind.Group == "extensions" && fqKind.Kind == "ThirdPartyResourceData" {
			fqKindToRegister = a.group.GroupVersion.WithKind(fqKind.Kind)
		}
	}
	if fqKindToRegister.Empty() {
		return schema.GroupVersionKind{}, fmt.Errorf("unable to locate fully qualified kind for %v: found %v when registering for %v", reflect.TypeOf(object), fqKinds, a.group.GroupVersion)
	}
	return fqKindToRegister, nil
}

// restMapping returns rest mapper for the resource provided by DynamicStorage.
func (a *EventsAPIInstaller) restMapping() (*meta.RESTMapping, error) {
	// subresources must have parent resources, and follow the namespacing rules of their parent.
	// So get the storage of the resource (which is the parent resource in case of subresources)
	fqKindToRegister, err := a.getResourceKind(a.group.DynamicStorage)
	if err != nil {
		return nil, fmt.Errorf("unable to locate fully qualified kind for mapper resource for dynamic storage: %v", err)
	}
	return a.group.Mapper.RESTMapping(fqKindToRegister.GroupKind(), fqKindToRegister.Version)
}

func addParams(route *restful.RouteBuilder, params []*restful.Parameter) {
	for _, param := range params {
		route.Param(param)
	}
}

// addObjectParams converts a runtime.Object into a set of go-restful Param() definitions on the route.
// The object must be a pointer to a struct; only fields at the top level of the struct that are not
// themselves interfaces or structs are used; only fields with a json tag that is non empty (the standard
// Go JSON behavior for omitting a field) become query parameters. The name of the query parameter is
// the JSON field name. If a description struct tag is set on the field, that description is used on the
// query parameter. In essence, it converts a standard JSON top level object into a query param schema.
func addObjectParams(ws *restful.WebService, route *restful.RouteBuilder, obj interface{}) error {
	sv, err := conversion.EnforcePtr(obj)
	if err != nil {
		return err
	}
	st := sv.Type()
	switch st.Kind() {
	case reflect.Struct:
		for i := 0; i < st.NumField(); i++ {
			name := st.Field(i).Name
			sf, ok := st.FieldByName(name)
			if !ok {
				continue
			}
			switch sf.Type.Kind() {
			case reflect.Interface, reflect.Struct:
			case reflect.Ptr:
				// TODO: This is a hack to let metav1.Time through. This needs to be fixed in a more generic way eventually. bug #36191
				if (sf.Type.Elem().Kind() == reflect.Interface || sf.Type.Elem().Kind() == reflect.Struct) && strings.TrimPrefix(sf.Type.String(), "*") != "metav1.Time" {
					continue
				}
				fallthrough
			default:
				jsonTag := sf.Tag.Get("json")
				if len(jsonTag) == 0 {
					continue
				}
				jsonName := strings.SplitN(jsonTag, ",", 2)[0]
				if len(jsonName) == 0 {
					continue
				}

				var desc string
				if docable, ok := obj.(documentable); ok {
					desc = docable.SwaggerDoc()[jsonName]
				}
				route.Param(ws.QueryParameter(jsonName, desc).DataType(typeToJSON(sf.Type.String())))
			}
		}
	}
	return nil
}

// TODO: this is incomplete, expand as needed.
// Convert the name of a golang type to the name of a JSON type
func typeToJSON(typeName string) string {
	switch typeName {
	case "bool", "*bool":
		return "boolean"
	case "uint8", "*uint8", "int", "*int", "int32", "*int32", "int64", "*int64", "uint32", "*uint32", "uint64", "*uint64":
		return "integer"
	case "float64", "*float64", "float32", "*float32":
		return "number"
	case "metav1.Time", "*metav1.Time":
		return "string"
	case "byte", "*byte":
		return "string"
	case "v1.DeletionPropagation", "*v1.DeletionPropagation":
		return "string"

	// TODO: Fix these when go-restful supports a way to specify an array query param:
	// https://github.com/emicklei/go-restful/issues/225
	case "[]string", "[]*string":
		return "string"
	case "[]int32", "[]*int32":
		return "integer"

	default:
		return typeName
	}
}

// rootScopeNaming reads only names from a request and ignores namespaces. It implements ScopeNamer
// for root scoped resources.
type rootScopeNaming struct {
	scope meta.RESTScope
	runtime.SelfLinker
	pathPrefix string
	pathSuffix string
}

// rootScopeNaming implements ScopeNamer
var _ handlers.ScopeNamer = rootScopeNaming{}

// Namespace returns an empty string because root scoped objects have no namespace.
func (n rootScopeNaming) Namespace(req *restful.Request) (namespace string, err error) {
	return "", nil
}

// Name returns the name from the path and an empty string for namespace, or an error if the
// name is empty.
func (n rootScopeNaming) Name(req *restful.Request) (namespace, name string, err error) {
	name = req.PathParameter("name")
	if len(name) == 0 {
		return "", "", errEmptyName
	}
	return "", name, nil
}

// GenerateLink returns the appropriate path and query to locate an object by its canonical path.
func (n rootScopeNaming) GenerateLink(req *restful.Request, obj runtime.Object) (uri string, err error) {
	_, name, err := n.ObjectName(obj)
	if err != nil {
		return "", err
	}
	if len(name) == 0 {
		_, name, err = n.Name(req)
		if err != nil {
			return "", err
		}
	}
	return n.pathPrefix + url.QueryEscape(name) + n.pathSuffix, nil
}

// GenerateListLink returns the appropriate path and query to locate a list by its canonical path.
func (n rootScopeNaming) GenerateListLink(req *restful.Request) (uri string, err error) {
	if len(req.Request.URL.RawPath) > 0 {
		return req.Request.URL.RawPath, nil
	}
	return req.Request.URL.EscapedPath(), nil
}

// ObjectName returns the name set on the object, or an error if the
// name cannot be returned. Namespace is empty
// TODO: distinguish between objects with name/namespace and without via a specific error.
func (n rootScopeNaming) ObjectName(obj runtime.Object) (namespace, name string, err error) {
	name, err = n.SelfLinker.Name(obj)
	if err != nil {
		return "", "", err
	}
	if len(name) == 0 {
		return "", "", errEmptyName
	}
	return "", name, nil
}

// scopeNaming returns naming information from a request. It implements ScopeNamer for
// namespace scoped resources.
type scopeNaming struct {
	scope meta.RESTScope
	runtime.SelfLinker
	itemPathFn    func(name, namespace string) bytes.Buffer
	allNamespaces bool
}

// scopeNaming implements ScopeNamer
var _ handlers.ScopeNamer = scopeNaming{}

// Namespace returns the namespace from the path or the default.
func (n scopeNaming) Namespace(req *restful.Request) (namespace string, err error) {
	if n.allNamespaces {
		return "", nil
	}
	namespace = req.PathParameter(n.scope.ArgumentName())
	if len(namespace) == 0 {
		// a URL was constructed without the namespace, or this method was invoked
		// on an object without a namespace path parameter.
		return "", fmt.Errorf("no namespace parameter found on request")
	}
	return namespace, nil
}

// Name returns the name from the path, the namespace (or default), or an error if the
// name is empty.
func (n scopeNaming) Name(req *restful.Request) (namespace, name string, err error) {
	namespace, _ = n.Namespace(req)
	name = req.PathParameter("name")
	if len(name) == 0 {
		return "", "", errEmptyName
	}
	return
}

// GenerateLink returns the appropriate path and query to locate an object by its canonical path.
func (n scopeNaming) GenerateLink(req *restful.Request, obj runtime.Object) (uri string, err error) {
	namespace, name, err := n.ObjectName(obj)
	if err != nil {
		return "", err
	}
	if len(namespace) == 0 && len(name) == 0 {
		namespace, name, err = n.Name(req)
		if err != nil {
			return "", err
		}
	}
	if len(name) == 0 {
		return "", errEmptyName
	}
	result := n.itemPathFn(name, namespace)
	return result.String(), nil
}

// GenerateListLink returns the appropriate path and query to locate a list by its canonical path.
func (n scopeNaming) GenerateListLink(req *restful.Request) (uri string, err error) {
	if len(req.Request.URL.RawPath) > 0 {
		return req.Request.URL.RawPath, nil
	}
	return req.Request.URL.EscapedPath(), nil
}

// ObjectName returns the name and namespace set on the object, or an error if the
// name cannot be returned.
// TODO: distinguish between objects with name/namespace and without via a specific error.
func (n scopeNaming) ObjectName(obj runtime.Object) (namespace, name string, err error) {
	name, err = n.SelfLinker.Name(obj)
	if err != nil {
		return "", "", err
	}
	namespace, err = n.SelfLinker.Namespace(obj)
	if err != nil {
		return "", "", err
	}
	return namespace, name, err
}

// An interface to see if an object supports swagger documentation as a method
type documentable interface {
	SwaggerDoc() map[string]string
}

// errEmptyName is returned when API requests do not fill the name section of the path.
var errEmptyName = errors.NewBadRequest("name must be provided")
