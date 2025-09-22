import { useQuery } from '@apollo/client';
import { DESCRIBE_CK } from '@/graphql';
import type { DescribeCodeKarma } from '@/types';

export const useDescribeCodeKarma = () => {
  const { data, loading, error } = useQuery<DescribeCodeKarma>(DESCRIBE_CK, {
    pollInterval: 5000,
  });

  return {
    data: data?.describeCodeKarma,
    loading,
    error,
  };
};
