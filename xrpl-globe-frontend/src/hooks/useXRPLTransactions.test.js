import { act, renderHook, waitFor } from '@testing-library/react';
import { useXRPLTransactions } from './useXRPLTransactions';

function createMockWebSocket() {
  class MockWebSocket {
    static instances = [];
    static CONNECTING = 0;
    static OPEN = 1;
    static CLOSING = 2;
    static CLOSED = 3;

    constructor(url) {
      this.url = url;
      this.readyState = MockWebSocket.CONNECTING;
      MockWebSocket.instances.push(this);
    }

    close() {
      this.readyState = MockWebSocket.CLOSED;
    }
  }

  return MockWebSocket;
}

describe('useXRPLTransactions', () => {
  const originalWebSocket = global.WebSocket;
  const originalFetch = global.fetch;

  afterEach(() => {
    global.WebSocket = originalWebSocket;
    global.fetch = originalFetch;
    jest.clearAllMocks();
    jest.useRealTimers();
  });

  test('parses transactions and caps recent list at 1000', async () => {
    const MockWebSocket = createMockWebSocket();
    global.WebSocket = MockWebSocket;
    global.fetch = jest.fn().mockResolvedValue({ ok: true });

    const { result, unmount } = renderHook(() =>
      useXRPLTransactions('ws://localhost:8080/transactions', 'http://localhost:8080/health')
    );

    await waitFor(() => expect(MockWebSocket.instances).toHaveLength(1));

    const ws = MockWebSocket.instances[0];

    for (let i = 0; i < 1002; i++) {
      const tx = {
        hash: `hash-${i}`,
        amount: String((i + 1) * 1_000_000),
        transaction_type: 'Payment',
        locations: [
          { latitude: 10, longitude: 20 },
          { latitude: 30, longitude: 40 }
        ]
      };
      act(() => {
        ws.onmessage({ data: JSON.stringify(tx) });
      });
    }

    await waitFor(() => expect(result.current.transactions).toHaveLength(1000));
    expect(result.current.transactions[0].id).toBe('hash-1001');
    expect(result.current.transactions[result.current.transactions.length - 1].id).toBe('hash-2');
    expect(result.current.transactions[0].amountXRP).toBe(1002);
    expect(result.current.transactions[0].geoPoints).toHaveLength(2);

    unmount();
  });

  test('does not create websocket after unmount while waiting for backend readiness', async () => {
    jest.useFakeTimers();

    const MockWebSocket = createMockWebSocket();
    global.WebSocket = MockWebSocket;
    global.fetch = jest
      .fn()
      .mockResolvedValueOnce({ ok: false })
      .mockResolvedValue({ ok: true });

    const { unmount } = renderHook(() =>
      useXRPLTransactions('ws://localhost:8080/transactions', 'http://localhost:8080/health')
    );

    await act(async () => {
      await Promise.resolve();
    });

    unmount();

    await act(async () => {
      jest.advanceTimersByTime(1000);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(MockWebSocket.instances).toHaveLength(0);
  });
});
