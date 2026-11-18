import { expect, it, vi } from 'vitest';
import { accountApi } from './account';

vi.mock('../analytics', () => ({ track: vi.fn() }));

it('opens checkout even when reading sessionStorage throws', async () => {
  const descriptor = Object.getOwnPropertyDescriptor(globalThis, 'sessionStorage');
  Object.defineProperty(globalThis, 'sessionStorage', {
    configurable: true,
    get() {
      throw new DOMException('Storage blocked', 'SecurityError');
    },
  });
  const requestData = vi.fn().mockResolvedValue({ url: 'https://checkout.stripe.com/test' });
  try {
    const api = accountApi({ baseUrl: '', call: vi.fn(), request: vi.fn(), requestData });
    await expect(api.billingCheckout()).resolves.toEqual({ url: 'https://checkout.stripe.com/test' });
    expect(requestData).toHaveBeenCalledWith('/api/v1/billing/checkout');
  } finally {
    if (descriptor) Object.defineProperty(globalThis, 'sessionStorage', descriptor);
    else Reflect.deleteProperty(globalThis, 'sessionStorage');
  }
});
