export function safeRedirect(path) {
  if (window.location.pathname === path) {
    return true;
  }

  window.location.replace(path);
}
