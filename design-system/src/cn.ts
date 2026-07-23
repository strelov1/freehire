import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';

/** Merge Tailwind class lists, resolving conflicts (last wins). */
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}
