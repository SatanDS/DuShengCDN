import { useId, type SVGProps } from 'react';

import { cn } from '@/lib/utils/cn';

export function MapleMark({ className, ...props }: SVGProps<SVGSVGElement>) {
  const id = useId().replace(/:/g, '');
  const gradientId = `satan-maple-gradient-${id}`;
  const stemId = `satan-maple-stem-${id}`;

  return (
    <svg
      viewBox="0 0 120 120"
      role="img"
      aria-label="SatanDu maple mark"
      className={cn('block', className)}
      {...props}
    >
      <defs>
        <linearGradient
          id={gradientId}
          x1="7"
          y1="24"
          x2="111"
          y2="97"
          gradientUnits="userSpaceOnUse"
        >
          <stop offset="0" stopColor="#0038ff" />
          <stop offset="0.48" stopColor="#7b20d8" />
          <stop offset="1" stopColor="#ff007f" />
        </linearGradient>
        <linearGradient
          id={stemId}
          x1="42"
          y1="86"
          x2="83"
          y2="116"
          gradientUnits="userSpaceOnUse"
        >
          <stop offset="0" stopColor="#0a3bff" />
          <stop offset="1" stopColor="#f00091" />
        </linearGradient>
      </defs>
      <path
        fill={`url(#${gradientId})`}
        d="M60 5.5 72.2 35 99 23.6l-8.6 29.2 24.3 6.7-24.9 11.1 10.7 27.6-28.9-10.5L60 116 48.4 87.7 19.5 98.2l10.7-27.6L5.3 59.5l24.3-6.7L21 23.6 47.8 35 60 5.5Z"
      />
      <path
        fill="#ffffff"
        fillOpacity="0.15"
        d="m60 19.5 8.7 21.1c1.6 3.8 6 5.6 9.8 4l9.8-4.1-4.1 13.9c-1.1 3.7 1 7.5 4.7 8.5l10.6 2.9-11 4.9c-3.3 1.5-4.8 5.4-3.5 8.7l4.2 10.8-13.7-5c-3.7-1.4-7.9.5-9.4 4.2L60 104.3l-6.1-14.9c-1.5-3.7-5.7-5.6-9.4-4.2l-13.7 5L35 79.4c1.3-3.3-.2-7.2-3.5-8.7l-11-4.9 10.6-2.9c3.7-1 5.8-4.8 4.7-8.5l-4.1-13.9 9.8 4.1c3.8 1.6 8.2-.2 9.8-4L60 19.5Z"
      />
      <path
        d="M60 34v65"
        stroke={`url(#${stemId})`}
        strokeWidth="6"
        strokeLinecap="round"
      />
      <path
        d="M60 65 43 55M60 72l19-12M60 83 42 78M60 90l17-6"
        stroke="#ffffff"
        strokeOpacity="0.58"
        strokeWidth="4"
        strokeLinecap="round"
      />
    </svg>
  );
}
