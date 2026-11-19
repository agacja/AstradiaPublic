// Public stand-in for the two session helpers the product modules import.
// The production client module also declares the private API surface; none of
// it is reproduced here.
export const API_BASE: string = import.meta.env.VITE_API_URL || '/api';

export function getAuthToken(): string | null {
  return localStorage.getItem('auth_token');
}
