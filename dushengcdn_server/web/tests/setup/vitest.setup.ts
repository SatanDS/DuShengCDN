import '@testing-library/jest-dom/vitest';
import { vi } from 'vitest';

vi.mock('echarts-for-react', async () => {
  const React = await import('react');
  return {
    default: () =>
      React.createElement('div', { 'data-testid': 'echarts-mock' }),
  };
});

vi.mock('echarts-for-react/lib/core', async () => {
  const React = await import('react');
  return {
    default: () =>
      React.createElement('div', { 'data-testid': 'echarts-mock' }),
  };
});
