import React, { useState } from 'react';
import styled from 'styled-components';
import { WarningTriangleIcon } from '@/assets';
import { Tooltip } from '@/reuseable-components';
import theme from '@/styles/theme';

interface WarningIconTooltipProps {
  message: string;
  size?: number;
  iconColor?: string;
}

const WarningIconWrapper = styled.div<{ $isHovered: boolean }>`
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: help;
  padding: 3px;
  border-radius: 50%;
  transition: background-color 0.2s ease;
  background-color: ${({ $isHovered }) => 
    $isHovered ? `${theme.text.warning}20` : 'transparent'};
  
  &:hover {
    background-color: ${theme.text.warning}20;
  }
`;

/**
 * A warning icon with a tooltip that appears on hover
 * @param message - The tooltip message to display on hover
 * @param size - The size of the icon (default: 16)
 * @param iconColor - The color of the icon (default: warning yellow)
 */
export const WarningIconTooltip: React.FC<WarningIconTooltipProps> = ({ 
  message, 
  size = 16, 
  iconColor = theme.text.warning 
}) => {
  const [isHovered, setIsHovered] = useState(false);

  return (
    <Tooltip text={message}>
      <WarningIconWrapper 
        $isHovered={isHovered}
        onMouseEnter={() => setIsHovered(true)}
        onMouseLeave={() => setIsHovered(false)}
      >
        <WarningTriangleIcon 
          size={size} 
          fill={iconColor} 
        />
      </WarningIconWrapper>
    </Tooltip>
  );
}; 