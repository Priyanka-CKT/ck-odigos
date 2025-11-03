import { gql } from '@apollo/client';

export const GET_AGENT_STATUS = gql`
  query GetAgentStatus($serviceName: String!) {
    getAgentStatus(serviceName: $serviceName)
  }
`;

