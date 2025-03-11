import React from 'react';
import { SVG } from '@/assets';
import theme from '@/styles/theme';

export const OdigosLogoText: SVG = ({ size = 16, fill = theme.text.secondary, rotate = 0, onClick }) => {
  return (
    <svg width="75" height="75" viewBox="0 0 256 256" fill="none" xmlns="http://www.w3.org/2000/svg">
      <path d="M224 96C224 65.8301 224 50.7452 214.627 41.3726C205.255 32 190.17 32 160 32H96C65.8301 32 50.7452 32 41.3726 41.3726C32 50.7452 32 65.8301 32 96V160C32 190.17 32 205.255 41.3726 214.627C50.7452 224 65.8301 224 96 224H160C190.17 224 205.255 224 214.627 214.627C224 205.255 224 190.17 224 160V96Z" fill="black"/>
      <rect width="144" height="124" rx="24" transform="matrix(-1 0 0 1 208 48)" fill="#CDFF52"/>
      <path d="M160 80L176 100.513L160 120" stroke="black" stroke-width="14.4"/>
      <path d="M128 80L112 100.513L128 120" stroke="black" stroke-width="14.4"/>
    </svg>
  );
};
