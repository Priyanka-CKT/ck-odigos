'use client';
import { useCallback } from 'react';
import { useMutation, useLazyQuery } from '@apollo/client';
import { DISABLE_AGENT_STATUS, ENABLE_AGENT_STATUS } from '@/graphql/mutations/agentstatus';
import { GET_AGENT_STATUS } from '@/graphql/queries/agentstatus';
import { useAgentStatusStore, type AgentStatus } from '@/store/useAgentStatusStore';

export const useAgentStatusesGraphQL = () => {
  const { statuses, setStatus } = useAgentStatusStore();

  const [getAgentStatusQuery] = useLazyQuery(GET_AGENT_STATUS, {
    fetchPolicy: 'no-cache',
    onCompleted: (data) => {
      console.log('Agent status response:', data);
      // Status update is handled in getStatus function since we can't access variables here
    },
    onError: (error) => {
      console.error('Error fetching agent status:', error);
    },
  });

  const [disableAgentStatusMutation] = useMutation(DISABLE_AGENT_STATUS, {
    onCompleted: (data) => {
      if (data.disableAgentStatus) {
        console.log('Successfully disabled agent status');
      }
    },
    onError: (error) => {
      console.error('Error disabling agent status:', error);
    },
  });

  const [enableAgentStatusMutation] = useMutation(ENABLE_AGENT_STATUS, {
    onCompleted: (data) => {
      if (data.enableAgentStatus) {
        console.log('Successfully enabled agent status');
      }
    },
    onError: (error) => {
      console.error('Error enabling agent status:', error);
    },
  });

  const getStatus = useCallback(async (serviceName: string): Promise<AgentStatus> => {
    try {
      console.log(`Fetching agent status for service: ${serviceName}`);
      const { data } = await getAgentStatusQuery({
        variables: { serviceName },
      });
      
      const status = data?.getAgentStatus || 'unknown';
      const agentStatus: AgentStatus = status === 'disabled' ? 'disabled' : 
                                     status === 'enabled' ? 'enabled' : 'unknown';
      
      // Update the store with the fetched status
      setStatus(serviceName, agentStatus);
      console.log(`Updated agent status for ${serviceName}: ${agentStatus}`);
      return agentStatus;
    } catch (error) {
      console.error(`Failed to get agent status for ${serviceName}:`, error);
      setStatus(serviceName, 'unknown');
      return 'unknown';
    }
  }, [getAgentStatusQuery, setStatus]);

  const disable = useCallback(async (serviceName: string, podId?: string): Promise<boolean> => {
    try {
      console.log(`Disabling agent status for service: ${serviceName}, podId: ${podId || 'string'}`);
      const { data } = await disableAgentStatusMutation({
        variables: { 
          serviceName, 
          podId: podId || 'string' 
        },
      });
      
      const success = data?.disableAgentStatus || false;
      console.log(`Disable result for ${serviceName}: ${success}`);
      
      if (success) {
        // Optimistically update the UI immediately
        setStatus(serviceName, 'disabled');
        // Then fetch the latest status from the API to confirm
        console.log(`Refetching status for ${serviceName} after disable`);
        await getStatus(serviceName);
      }
      
      return success;
    } catch (error) {
      console.error(`Failed to disable agent status for ${serviceName}:`, error);
      return false;
    }
  }, [disableAgentStatusMutation, getStatus]);

  const enable = useCallback(async (serviceName: string, podId?: string): Promise<boolean> => {
    try {
      console.log(`Enabling agent status for service: ${serviceName}, podId: ${podId || 'string'}`);
      const { data } = await enableAgentStatusMutation({
        variables: { 
          serviceName, 
          podId: podId || 'string' 
        },
      });
      
      const success = data?.enableAgentStatus || false;
      console.log(`Enable result for ${serviceName}: ${success}`);
      
      if (success) {
        // Optimistically update the UI immediately
        setStatus(serviceName, 'enabled');
        // Then fetch the latest status from the API to confirm
        console.log(`Refetching status for ${serviceName} after enable`);
        await getStatus(serviceName);
      }
      
      return success;
    } catch (error) {
      console.error(`Failed to enable agent status for ${serviceName}:`, error);
      return false;
    }
  }, [enableAgentStatusMutation, getStatus]);

  return { statuses, getStatus, disable, enable };
};
