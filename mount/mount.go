package mount

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/malcolmston/fastmcp"
)

// ResourcePrefixStyle selects how a child resource URI is rewritten when it is
// exposed on the parent under a prefix.
type ResourcePrefixStyle int

const (
	// PrefixPath inserts the prefix as the first path segment, rewriting
	// "scheme://rest" to "scheme://{prefix}/rest". This is FastMCP 2.x's default.
	PrefixPath ResourcePrefixStyle = iota
	// PrefixProtocol prepends the prefix to the scheme, producing the legacy
	// "{prefix}+scheme://rest" form.
	PrefixProtocol
)

// CollisionPolicy decides what happens when a prefixed component would overwrite
// a component already registered on the parent.
type CollisionPolicy int

const (
	// CollisionError aborts the whole operation with an error on the first
	// collision. It is the default.
	CollisionError CollisionPolicy = iota
	// CollisionSkip silently leaves the existing parent component in place and
	// does not register the colliding child component.
	CollisionSkip
	// CollisionOverwrite registers the child component, replacing the existing
	// parent one.
	CollisionOverwrite
)

// MountConfig tunes how a child server is composed onto a parent. The zero value
// disables every category; start from [DefaultConfig] and adjust the fields you
// care about.
type MountConfig struct {
	// Separator joins the prefix and a tool or prompt name. Empty is normalised
	// to "_".
	Separator string
	// ResourceStyle selects how resource and resource-template URIs are prefixed.
	ResourceStyle ResourcePrefixStyle
	// IncludeTools, IncludeResources and IncludePrompts select which categories
	// of component are composed. Resource templates follow IncludeResources.
	IncludeTools     bool
	IncludeResources bool
	IncludePrompts   bool
	// OnCollision decides how a name clash with an existing parent component is
	// handled.
	OnCollision CollisionPolicy
}

// DefaultConfig returns the configuration used by [Import] and [Mount]: an
// underscore separator, path-style resource prefixing, every category included,
// and [CollisionError] on a clash.
func DefaultConfig() MountConfig {
	return MountConfig{
		Separator:        "_",
		ResourceStyle:    PrefixPath,
		IncludeTools:     true,
		IncludeResources: true,
		IncludePrompts:   true,
		OnCollision:      CollisionError,
	}
}

// separator returns the effective separator, defaulting empty to "_".
func (c MountConfig) separator() string {
	if c.Separator == "" {
		return "_"
	}
	return c.Separator
}

// Mounted records the outcome of an [Import] or [Mount]: the prefix used, the
// child server, and the prefixed names or URIs registered on the parent for each
// category. Live is true for [Mount] (a live passthrough) and false for [Import]
// (a static copy).
type Mounted struct {
	Prefix            string
	Child             *fastmcp.Server
	Live              bool
	Tools             []string
	Resources         []string
	ResourceTemplates []string
	Prompts           []string
}

// Import performs a one-time static copy of the child's tools, resources,
// resource templates and prompts onto the parent, addressable under prefix. It
// uses [DefaultConfig]. See [ImportWithConfig] for options and the returned
// [Mounted] record.
func Import(parent, child *fastmcp.Server, prefix string) error {
	_, err := ImportWithConfig(parent, child, prefix, DefaultConfig())
	return err
}

// Mount establishes a live mount of the child onto the parent under prefix,
// registering passthrough handlers that dispatch to the child at call time. It
// uses [DefaultConfig]. See [MountWithConfig] for options and the returned
// [Mounted] record.
func Mount(parent, child *fastmcp.Server, prefix string) error {
	_, err := MountWithConfig(parent, child, prefix, DefaultConfig())
	return err
}

// ImportWithConfig is [Import] with an explicit [MountConfig], returning a
// [Mounted] record describing what was registered.
func ImportWithConfig(parent, child *fastmcp.Server, prefix string, cfg MountConfig) (*Mounted, error) {
	return compose(parent, child, prefix, cfg, false)
}

// MountWithConfig is [Mount] with an explicit [MountConfig], returning a
// [Mounted] record describing what was registered.
func MountWithConfig(parent, child *fastmcp.Server, prefix string, cfg MountConfig) (*Mounted, error) {
	return compose(parent, child, prefix, cfg, true)
}

// compose is the shared implementation behind Import and Mount. The live flag
// only affects the recorded Mounted.Live value; both modes register call-time
// passthrough handlers because the root package offers no way to extract a
// handler's internal logic.
func compose(parent, child *fastmcp.Server, prefix string, cfg MountConfig, live bool) (*Mounted, error) {
	if parent == nil || child == nil {
		return nil, fmt.Errorf("mount: parent and child servers must be non-nil")
	}
	if prefix == "" {
		return nil, fmt.Errorf("mount: prefix must not be empty")
	}
	if parent == child {
		return nil, fmt.Errorf("mount: cannot compose a server onto itself")
	}

	src := newDelegate(child)
	taken, err := existingNames(parent)
	if err != nil {
		return nil, err
	}

	rec := &Mounted{Prefix: prefix, Child: child, Live: live}

	if cfg.IncludeTools {
		if err := composeTools(parent, src, prefix, cfg, taken, rec); err != nil {
			return nil, err
		}
	}
	if cfg.IncludeResources {
		if err := composeResources(parent, src, prefix, cfg, taken, rec); err != nil {
			return nil, err
		}
		if err := composeTemplates(parent, src, prefix, cfg, taken, rec); err != nil {
			return nil, err
		}
	}
	if cfg.IncludePrompts {
		if err := composePrompts(parent, src, prefix, cfg, taken, rec); err != nil {
			return nil, err
		}
	}
	return rec, nil
}

// nameSets tracks the identifiers already registered on the parent, so
// collisions (including with components added earlier in the same operation) can
// be detected.
type nameSets struct {
	tools     map[string]struct{}
	resources map[string]struct{}
	templates map[string]struct{}
	prompts   map[string]struct{}
}

// existingNames snapshots the parent's currently registered identifiers.
func existingNames(parent *fastmcp.Server) (*nameSets, error) {
	d := newDelegate(parent)
	ns := &nameSets{
		tools:     map[string]struct{}{},
		resources: map[string]struct{}{},
		templates: map[string]struct{}{},
		prompts:   map[string]struct{}{},
	}
	tools, err := d.listTools()
	if err != nil {
		return nil, err
	}
	for _, t := range tools {
		ns.tools[t.Name] = struct{}{}
	}
	resources, err := d.listResources()
	if err != nil {
		return nil, err
	}
	for _, r := range resources {
		ns.resources[r.URI] = struct{}{}
	}
	templates, err := d.listTemplates()
	if err != nil {
		return nil, err
	}
	for _, t := range templates {
		ns.templates[t.URITemplate] = struct{}{}
	}
	prompts, err := d.listPrompts()
	if err != nil {
		return nil, err
	}
	for _, p := range prompts {
		ns.prompts[p.Name] = struct{}{}
	}
	return ns, nil
}

// resolve applies the collision policy to a candidate identifier. It reports
// whether the caller should proceed with registration and records the name as
// taken when it does.
func resolve(set map[string]struct{}, name, kind string, policy CollisionPolicy) (bool, error) {
	if _, clash := set[name]; clash {
		switch policy {
		case CollisionSkip:
			return false, nil
		case CollisionOverwrite:
			return true, nil
		default:
			return false, fmt.Errorf("mount: %s %q already registered on parent", kind, name)
		}
	}
	set[name] = struct{}{}
	return true, nil
}

// composeTools registers a delegating tool on the parent for each child tool.
func composeTools(parent *fastmcp.Server, src *delegate, prefix string, cfg MountConfig, taken *nameSets, rec *Mounted) error {
	tools, err := src.listTools()
	if err != nil {
		return err
	}
	for _, t := range tools {
		name := prefixName(prefix, t.Name, cfg.separator())
		ok, err := resolve(taken.tools, name, "tool", cfg.OnCollision)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		origin := t.Name
		parent.Tool(name, t.Description, func(ctx context.Context, args map[string]any) (any, error) {
			return src.callTool(ctx, origin, args)
		})
		rec.Tools = append(rec.Tools, name)
	}
	return nil
}

// composeResources registers a delegating static resource on the parent for each
// child resource.
func composeResources(parent *fastmcp.Server, src *delegate, prefix string, cfg MountConfig, taken *nameSets, rec *Mounted) error {
	resources, err := src.listResources()
	if err != nil {
		return err
	}
	for _, r := range resources {
		uri := prefixURI(r.URI, prefix, cfg.ResourceStyle)
		ok, err := resolve(taken.resources, uri, "resource", cfg.OnCollision)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		origin := r.URI
		parent.Resource(uri, r.Name, r.Description, r.MIMEType, func(ctx context.Context) (string, error) {
			return src.readResource(ctx, origin)
		})
		rec.Resources = append(rec.Resources, uri)
	}
	return nil
}

// composeTemplates registers a delegating resource template on the parent for
// each child template, reconstructing the child's concrete URI from the matched
// parameters at read time.
func composeTemplates(parent *fastmcp.Server, src *delegate, prefix string, cfg MountConfig, taken *nameSets, rec *Mounted) error {
	templates, err := src.listTemplates()
	if err != nil {
		return err
	}
	for _, t := range templates {
		tmpl := prefixURI(t.URITemplate, prefix, cfg.ResourceStyle)
		ok, err := resolve(taken.templates, tmpl, "resource template", cfg.OnCollision)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		origin := t.URITemplate
		parent.ResourceTemplate(tmpl, t.Name, t.Description, t.MIMEType, func(ctx context.Context, params map[string]string) (string, error) {
			return src.readResource(ctx, expandTemplate(origin, params))
		})
		rec.ResourceTemplates = append(rec.ResourceTemplates, tmpl)
	}
	return nil
}

// composePrompts registers a delegating prompt on the parent for each child
// prompt.
func composePrompts(parent *fastmcp.Server, src *delegate, prefix string, cfg MountConfig, taken *nameSets, rec *Mounted) error {
	prompts, err := src.listPrompts()
	if err != nil {
		return err
	}
	for _, p := range prompts {
		name := prefixName(prefix, p.Name, cfg.separator())
		ok, err := resolve(taken.prompts, name, "prompt", cfg.OnCollision)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		origin := p.Name
		parent.Prompt(name, p.Description, func(ctx context.Context, args map[string]string) ([]fastmcp.PromptMessage, error) {
			return src.getPrompt(ctx, origin, args)
		}, p.Arguments...)
		rec.Prompts = append(rec.Prompts, name)
	}
	return nil
}

// prefixName joins a prefix and a tool or prompt name with the separator.
func prefixName(prefix, name, sep string) string {
	return prefix + sep + name
}

// prefixURI rewrites a resource URI (or URI template) to carry the prefix in the
// selected style. A URI without a "scheme://" marker is prefixed path-style
// against its leading segment.
func prefixURI(uri, prefix string, style ResourcePrefixStyle) string {
	idx := strings.Index(uri, "://")
	if idx < 0 {
		return prefix + "/" + uri
	}
	scheme := uri[:idx]
	rest := uri[idx+len("://"):]
	if style == PrefixProtocol {
		return prefix + "+" + scheme + "://" + rest
	}
	return scheme + "://" + prefix + "/" + rest
}

// templateVar matches a "{name}" placeholder in a URI template.
var templateVar = regexp.MustCompile(`\{([^}]+)\}`)

// expandTemplate substitutes params into a URI template, reproducing the child's
// concrete resource URI from the parameters the parent extracted.
func expandTemplate(tmpl string, params map[string]string) string {
	return templateVar.ReplaceAllStringFunc(tmpl, func(m string) string {
		name := m[1 : len(m)-1]
		if v, ok := params[name]; ok {
			return v
		}
		return m
	})
}
