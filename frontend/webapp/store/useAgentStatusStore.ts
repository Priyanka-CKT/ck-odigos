import { create } from 'zustand';

export type AgentStatus = 'enabled' | 'disabled' | 'unknown';

type ServiceKey = string; // typically namespace-qualified name or reportedName

interface AgentStatusState {
  statuses: Record<ServiceKey, AgentStatus>;
  setStatus: (service: ServiceKey, status: AgentStatus) => void;
  setStatuses: (items: Record<ServiceKey, AgentStatus>) => void;
  resetStatuses: () => void;
}

export const useAgentStatusStore = create<AgentStatusState>((set) => ({
  statuses: {},
  setStatus: (service, status) => set((state) => ({ statuses: { ...state.statuses, [service]: status } })),
  setStatuses: (items) => set(() => ({ statuses: items })),
  resetStatuses: () => set(() => ({ statuses: {} })),
}));




