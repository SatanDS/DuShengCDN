export type NavigationIconKey =
  | 'home'
  | 'traffic'
  | 'node'
  | 'website'
  | 'origin'
  | 'domain'
  | 'dns'
  | 'certificate'
  | 'proxy'
  | 'release'
  | 'log'
  | 'performance'
  | 'shield'
  | 'user'
  | 'setting';

export interface NavigationItem {
  href: string;
  label: string;
  icon: NavigationIconKey;
  minRole?: number;
  children?: NavigationItem[];
}
