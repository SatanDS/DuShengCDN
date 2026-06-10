import { describe, expect, it } from 'vitest';

import {
  buildNodeDockerInstallCommand,
  buildNodeInstallCommand,
} from '@/features/nodes/utils';

describe('node install command builders', () => {
  it('prompts for the agent token instead of embedding it in shell commands', () => {
    const command = buildNodeInstallCommand(
      "https://cdn.example.com/a b?x=';echo bad",
      'secret-agent-token',
    );

    expect(command).not.toContain('secret-agent-token');
    expect(command).toContain('IFS= read -r agent_token');
    expect(command).toContain('--agent-token-file "$token_file"');
    expect(command).toContain(
      "--server-url 'https://cdn.example.com/a b?x='\\'';echo bad'",
    );
  });

  it('mounts Docker agent token as a file secret', () => {
    const command = buildNodeDockerInstallCommand(
      'https://cdn.example.com',
      'secret-agent-token',
    );

    expect(command).not.toContain('secret-agent-token');
    expect(command).toContain(
      '-v "$token_file":/run/secrets/dushengcdn_agent_token:ro',
    );
    expect(command).toContain(
      '-e DUSHENGCDN_AGENT_TOKEN_FILE=/run/secrets/dushengcdn_agent_token',
    );
    expect(command).not.toContain('DUSHENGCDN_AGENT_TOKEN=');
  });
});
