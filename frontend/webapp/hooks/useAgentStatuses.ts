'use client';
import { useCallback } from 'react';
import { useConfig } from '@/hooks/config';
import { useAgentStatusStore, type AgentStatus } from '@/store/useAgentStatusStore';

async function safeFetch(input: RequestInfo | URL, init?: RequestInit) {
  try {
    const res = await fetch(input, init);
    return res;
  } catch (e) {
    return undefined;
  }
}

export const useAgentStatuses = () => {
  const { statuses, setStatus } = useAgentStatusStore();
  const { data: config } = useConfig();

  const buildAgentStatusUrl = useCallback((serviceName: string) => {
    const nexusEndpoint = config?.nexusEndpoint;
    if (!nexusEndpoint) return '';
    return `${nexusEndpoint}/api/agent-status/${encodeURIComponent(serviceName)}`;
  }, [config?.nexusEndpoint]);

  const getStatus = useCallback(async (serviceName: string): Promise<AgentStatus> => {
    const url = buildAgentStatusUrl(serviceName);
    if (!url) return 'unknown';

    const res = await safeFetch(url, { method: 'GET', headers: { accept: '*/*' } });
    if (!res || !res.ok) return 'unknown';
    try {
      const json = (await res.json()) as { status?: AgentStatus };
      const value: AgentStatus = (json.status as AgentStatus) || 'unknown';
      setStatus(serviceName, value);
      return value;
    } catch {
      return 'unknown';
    }
  }, [buildAgentStatusUrl, setStatus]);

  const disable = useCallback(async (serviceName: string, podId?: string) => {
    const url = buildAgentStatusUrl(serviceName);
    if (!url) return false;
    const body = JSON.stringify({ status: 'disabled', podId: podId || 'string' });
    const res = await safeFetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', accept: '*/*' },
      body,
    });
    if (res && res.ok) {
      setStatus(serviceName, 'disabled');
      return true;
    }
    return false;
  }, [buildAgentStatusUrl, setStatus]);

  return { statuses, getStatus, disable };
};


