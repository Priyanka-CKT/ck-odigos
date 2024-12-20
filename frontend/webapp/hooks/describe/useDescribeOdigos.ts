import { useQuery } from '@apollo/client';
import { DESCRIBE_CK } from '@/graphql';
import type { DescribeOdigos } from '@/types';

export const useDescribeOdigos = () => {
  const { data, loading, error } = useQuery<DescribeOdigos>(DESCRIBE_CK, {
    pollInterval: 5000,
  });

  return {
    data: data?.describeOdigos,
    loading,
    error,
  };
};
