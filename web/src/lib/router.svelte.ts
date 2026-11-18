// Minimal hash router: #/, #/site/vinted, #/site/vinted/searches, #/activity, #/settings

export const route = $state({ path: '/', parts: [] as string[] });

function parse() {
  const path = location.hash.replace(/^#/, '') || '/';
  route.path = path;
  route.parts = path.split('/').filter(Boolean);
}

window.addEventListener('hashchange', parse);
parse();

export function navigate(path: string) {
  location.hash = path;
}

/** True when the current route starts with the given path. */
export function isActive(path: string): boolean {
  if (path === '/') return route.path === '/';
  return route.path === path || route.path.startsWith(path + '/');
}
