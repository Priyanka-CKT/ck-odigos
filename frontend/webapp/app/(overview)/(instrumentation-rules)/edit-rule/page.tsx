'use client';
import React, { useEffect } from 'react';
import { useRouter } from 'next/navigation';
import styled from 'styled-components';
import { Text } from '@/reuseable-components';

const Container = styled.div`
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 2rem;
  height: calc(100vh - 176px);
  text-align: center;
`;

export default function EditRulePage() {
  const router = useRouter();

  useEffect(() => {
    // Redirect to overview page after showing message briefly
    const redirectTimer = setTimeout(() => {
      router.push('/overview');
    }, 3000);

    return () => clearTimeout(redirectTimer);
  }, [router]);

  return (
    <Container>
      <Text size={24} weight={600} style={{ marginBottom: '1rem' }}>
        InstrumentationRules Feature Removed
      </Text>
      <Text size={16} style={{ maxWidth: '600px', marginBottom: '1rem' }}>
        The InstrumentationRules feature has been removed from the system. All signals are now enabled by default.
      </Text>
      <Text size={14} style={{ opacity: 0.7 }}>
        Redirecting to overview page...
      </Text>
    </Container>
  );
} 