import { dashboardNavigation } from '@/lib/constants/navigation';
import type { NavigationItem } from '@/types/navigation';

const navigationPathAliases: Record<string, string> = {
  '/certificate': '/website/certificate',
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
  items = dashboardNavigation,
): boolean {
  const currentItem = getCurrentNavigationItem(pathname, items);
  return Boolean(currentItem && containsNavigationItem(item, currentItem));
}

export function flattenNavigationItems(
  items: NavigationItem[],
): NavigationItem[] {
  return items.flatMap((item) => [
    item,
    ...(item.children ? flattenNavigationItems(item.children) : []),
  ]);
}

export function filterNavigationItemsByRole(
  items: NavigationItem[],
  role: number,
): NavigationItem[] {
  return items
    .filter((item) => role >= (item.minRole ?? 0))
    .map((item) => {
      if (!item.children?.length) {
        return item;
      }

      return {
        ...item,
        children: filterNavigationItemsByRole(item.children, role),
      };
    });
}

export function getCurrentNavigationItem(
  pathname: string,
  items = dashboardNavigation,
): NavigationItem | undefined {
  type MatchCandidate = {
    item: NavigationItem;
    pathLength: number;
    searchParamCount: number;
    depth: number;
    order: number;
  };

  let bestMatch: MatchCandidate | undefined;
  let visitOrder = 0;

  const isBetterMatch = (
    candidate: MatchCandidate,
    current: MatchCandidate | undefined,
  ) => {
    if (!current) {
      return true;
    }

    if (candidate.searchParamCount !== current.searchParamCount) {
      return candidate.searchParamCount > current.searchParamCount;
    }

    if (candidate.pathLength !== current.pathLength) {
      return candidate.pathLength > current.pathLength;
    }

    if (candidate.depth !== current.depth) {
      return candidate.depth > current.depth;
    }

    return candidate.order < current.order;
  };

  const visitItems = (items: NavigationItem[], depth = 0) => {
    for (const item of items) {
      const order = visitOrder;
      visitOrder += 1;

      if (isPathActive(pathname, item.href)) {
        const candidate = {
          item,
          pathLength: normalizeNavigationPath(item.href).length,
          searchParamCount: getNavigationSearchParams(item.href).size,
          depth,
          order,
        };

        if (isBetterMatch(candidate, bestMatch)) {
          bestMatch = candidate;
        }
      }

      if (item.children) {
        visitItems(item.children, depth + 1);
      }
    }
  };

  visitItems(items);

  return bestMatch?.item;
}

function containsNavigationItem(
  item: NavigationItem,
  target: NavigationItem,
): boolean {
  return (
    item === target ||
    item.children?.some((child) => containsNavigationItem(child, target)) ||
    false
  );
}
