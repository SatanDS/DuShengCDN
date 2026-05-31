import { dashboardNavigation } from '@/lib/constants/navigation';
import type { NavigationItem } from '@/types/navigation';

const navigationPathAliases: Record<string, string> = {
  '/certificate': '/website',
  '/website/certificate': '/website',
};

function normalizeNavigationPath(pathname: string) {
  const [pathOnly] = pathname.split('?');
  return navigationPathAliases[pathOnly] ?? pathOnly;
}

function getNavigationSearchParams(pathname: string) {
  const queryStart = pathname.indexOf('?');
  if (queryStart < 0) {
    return new URLSearchParams();
  }
  return new URLSearchParams(pathname.slice(queryStart + 1));
}

export function isPathActive(pathname: string, href: string) {
  const normalizedPathname = normalizeNavigationPath(pathname);
  const normalizedHref = normalizeNavigationPath(href);
  const hrefSearchParams = getNavigationSearchParams(href);

  if (hrefSearchParams.size > 0) {
    const pathnameSearchParams = getNavigationSearchParams(pathname);
    const allParamsMatch = Array.from(hrefSearchParams.entries()).every(
      ([key, value]) => pathnameSearchParams.get(key) === value,
    );
    const pathMatches =
      normalizedPathname === normalizedHref ||
      normalizedPathname.startsWith(`${normalizedHref}/`);
    return pathMatches && allParamsMatch;
  }

  if (normalizedHref === '/') {
    return normalizedPathname === '/';
  }

  return (
    normalizedPathname === normalizedHref ||
    normalizedPathname.startsWith(`${normalizedHref}/`)
  );
}

export function isNavigationItemActive(
  pathname: string,
  item: NavigationItem,
): boolean {
  return (
    isPathActive(pathname, item.href) ||
    item.children?.some((child) => isNavigationItemActive(pathname, child)) ||
    false
  );
}

export function flattenNavigationItems(
  items: NavigationItem[],
): NavigationItem[] {
  return items.flatMap((item) => [
    item,
    ...(item.children ? flattenNavigationItems(item.children) : []),
  ]);
}

export function getCurrentNavigationItem(
  pathname: string,
): NavigationItem | undefined {
  const findMatch = (items: NavigationItem[]): NavigationItem | undefined => {
    for (const item of items) {
      if (item.children) {
        const childMatch = findMatch(item.children);
        if (childMatch) {
          return childMatch;
        }
      }

      if (isPathActive(pathname, item.href)) {
        return item;
      }
    }

    return undefined;
  };

  return findMatch(dashboardNavigation);
}
