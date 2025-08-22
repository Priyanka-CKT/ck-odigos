import { gql } from '@apollo/client';

export const DISABLE_AGENT_STATUS = gql`
  mutation DisableAgentStatus($serviceName: String!, $podId: String) {
    disableAgentStatus(serviceName: $serviceName, podId: $podId)
  }
`;

export const ENABLE_AGENT_STATUS = gql`
  mutation EnableAgentStatus($serviceName: String!, $podId: String) {
    enableAgentStatus(serviceName: $serviceName, podId: $podId)
  }
`;
