# Recipes for the UI's component browser, a dev-only Storybook (web/.storybook/).
# Storybook is a devDependency of web/ and nothing in the embedded dist or in
# `make test` uses it.

# Serve the component browser on http://localhost:6006 with hot reload.
serve-components: _install
    cd web && npm exec -- storybook dev -p 6006 --no-open

# Build the component browser statically into web/storybook-static, which checks it builds.
build-components: _install
    cd web && npm exec -- storybook build

# Install the web dependencies when Storybook isn't there yet.
_install:
    test -d web/node_modules/storybook || npm --prefix web ci
