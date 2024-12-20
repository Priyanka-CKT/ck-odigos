import React from 'react';
import { SVG } from '@/assets';
import theme from '@/styles/theme';

export const OdigosLogoText: SVG = ({ size = 16, fill = theme.text.secondary, rotate = 0, onClick }) => {
  return (
    <svg width="81" height="82" viewBox="0 0 81 82" fill="none" xmlns="http://www.w3.org/2000/svg">
   <g filter="url(#filter0_ddd_2314_4528)">
      <rect x="3" y="2" width="72" height="72" rx="11.52" fill="url(#paint0_linear_2314_4528)"/>
      <rect x="3.18" y="2.18" width="71.64" height="71.64" rx="11.34" stroke="#BACCFF" stroke-width="0.36"/>
      <path fill-rule="evenodd" clip-rule="evenodd" d="M42.0371 22.2305V53.7665H46.7027V39.5537H50.8067L57.5459 53.7665L61.8252 50.5451L54.9971 37.3505L61.4652 26.587L57.3299 22.2305L50.8931 35.3633H46.7027V22.2305H42.0371Z" fill="black"/>
      <path d="M25.6374 54.1992C23.679 54.1992 21.9654 53.8392 20.4966 53.1192C19.0566 52.3704 17.9334 51.3048 17.127 49.9224C16.3494 48.5112 15.9606 46.8696 15.9606 44.9976V31.0008C15.9606 29.1 16.3494 27.4584 17.127 26.076C17.9334 24.6936 19.0566 23.6424 20.4966 22.9224C21.9654 22.1736 23.679 21.7992 25.6374 21.7992C27.5958 21.7992 29.295 22.1736 30.735 22.9224C32.175 23.6712 33.2838 24.7368 34.0614 26.1192C34.8678 27.4728 34.8247 28.1067 34.8247 30.0075L30.6054 31.0008C30.6054 29.3592 30.1734 28.1064 29.3094 27.2424C28.4454 26.3784 27.2214 25.9464 25.6374 25.9464C24.0534 25.9464 22.815 26.3784 21.9222 27.2424C21.0582 28.1064 20.6262 29.3448 20.6262 30.9576V44.9976C20.6262 46.6104 21.0582 47.8632 21.9222 48.756C22.815 49.62 24.0534 50.052 25.6374 50.052C27.2214 50.052 28.4454 49.62 29.3094 48.756C30.1734 47.8632 30.6054 46.6104 30.6054 44.9976L35.1847 43.8675C35.1847 45.7395 34.8678 48.4968 34.0614 49.8792C33.2838 51.2616 32.175 52.3272 30.735 53.076C29.295 53.8248 27.5958 54.1992 25.6374 54.1992Z" fill="black"/>
   </g>
   <defs>
      <filter id="filter0_ddd_2314_4528" x="0.12" y="0.56" width="80.64" height="80.64" filterUnits="userSpaceOnUse" color-interpolation-filters="sRGB">
         <feFlood flood-opacity="0" result="BackgroundImageFix"/>
         <feColorMatrix in="SourceAlpha" type="matrix" values="0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 127 0" result="hardAlpha"/>
         <feOffset dx="1.44" dy="2.88"/>
         <feGaussianBlur stdDeviation="2.16"/>
         <feComposite in2="hardAlpha" operator="out"/>
         <feColorMatrix type="matrix" values="0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0.12 0"/>
         <feBlend mode="normal" in2="BackgroundImageFix" result="effect1_dropShadow_2314_4528"/>
         <feColorMatrix in="SourceAlpha" type="matrix" values="0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 127 0" result="hardAlpha"/>
         <feMorphology radius="0.72" operator="dilate" in="SourceAlpha" result="effect2_dropShadow_2314_4528"/>
         <feOffset/>
         <feComposite in2="hardAlpha" operator="out"/>
         <feColorMatrix type="matrix" values="0 0 0 0 0.573581 0 0 0 0 0.682677 0 0 0 0 0.991783 0 0 0 1 0"/>
         <feBlend mode="normal" in2="effect1_dropShadow_2314_4528" result="effect2_dropShadow_2314_4528"/>
         <feColorMatrix in="SourceAlpha" type="matrix" values="0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 127 0" result="hardAlpha"/>
         <feOffset dy="0.36"/>
         <feGaussianBlur stdDeviation="0.36"/>
         <feComposite in2="hardAlpha" operator="out"/>
         <feColorMatrix type="matrix" values="0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0.2 0"/>
         <feBlend mode="normal" in2="effect2_dropShadow_2314_4528" result="effect3_dropShadow_2314_4528"/>
         <feBlend mode="normal" in="SourceGraphic" in2="effect3_dropShadow_2314_4528" result="shape"/>
      </filter>
      <linearGradient id="paint0_linear_2314_4528" x1="39" y1="31.34" x2="39" y2="117.92" gradientUnits="userSpaceOnUse">
         <stop stop-color="white"/>
         <stop offset="1" stop-color="#BACCFF"/>
      </linearGradient>
   </defs>
</svg>

  );
};
