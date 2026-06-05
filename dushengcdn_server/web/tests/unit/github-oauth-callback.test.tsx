import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, waitFor } from '@testing-library/react';
import { StrictMode, type ReactNode } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { GitHubOAuthCallback } from '@/features/auth/components/github-oauth-callback';

const replaceMock = vi.fn();
const setUserMock = vi.fn();
const exchangeGitHubCodeMock = vi.fn();
let searchParams = new URLSearchParams('code=github-code-123&state=oauth-state-123&legacy=1');

vi.mock('next/navigation', () => ({
  useRouter: () => ({
    replace: replaceMock,
  }),
  useSearchParams: () => searchParams,
  usePathname: () => '/oauth/github',
}));

vi.mock('@/components/providers/auth-provider', () => ({
  useAuth: () => ({
    setUser: setUserMock,
  }),
}));

vi.mock('@/features/auth/api/auth', () => ({
  exchangeGitHubCode: (code: string, state: string) =>
    exchangeGitHubCodeMock(code, state),
  exchangeOAuthCode: vi.fn(),
}));

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
      mutations: {
        retry: false,
      },
    },
  });
}

function renderWithProviders(ui: ReactNode) {
  const queryClient = createQueryClient();

  return render(
    <StrictMode>
      <QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>
    </StrictMode>,
  );
}

describe('GitHubOAuthCallback', () => {
  beforeEach(() => {
    replaceMock.mockReset();
    setUserMock.mockReset();
    exchangeGitHubCodeMock.mockReset();
    searchParams = new URLSearchParams('code=github-code-123&state=oauth-state-123&legacy=1');
    exchangeGitHubCodeMock.mockResolvedValue({
      id: 1,
      username: 'github-user',
      role: 1,
      status: 1,
    });
  });

  it('exchanges the same GitHub code only once', async () => {
    const view = renderWithProviders(<GitHubOAuthCallback />);

    await waitFor(() => {
      expect(exchangeGitHubCodeMock).toHaveBeenCalledTimes(1);
    });

    view.rerender(
      <StrictMode>
        <QueryClientProvider client={createQueryClient()}>
          <GitHubOAuthCallback />
        </QueryClientProvider>
      </StrictMode>,
    );

    await waitFor(() => {
      expect(replaceMock).toHaveBeenCalledWith('/');
    });

    expect(exchangeGitHubCodeMock).toHaveBeenCalledTimes(1);
    expect(exchangeGitHubCodeMock).toHaveBeenCalledWith('github-code-123', 'oauth-state-123');
    expect(setUserMock).toHaveBeenCalledTimes(1);
  });

  it('does not exchange a legacy GitHub code without state', async () => {
    searchParams = new URLSearchParams('code=github-code-123&legacy=1');

    renderWithProviders(<GitHubOAuthCallback />);

    await waitFor(() => {
      expect(exchangeGitHubCodeMock).not.toHaveBeenCalled();
    });
  });
});
