import { io, Socket } from 'socket.io-client';

export interface WebSocketMessage {
  type: string;
  data: any;
  time: string;
}

class WebSocketService {
  private socket: Socket | null = null;
  private listeners: Map<string, Array<(data: any) => void>> = new Map();

  connect(): void {
    if (this.socket?.connected) {
      return;
    }

    const wsUrl = process.env.REACT_APP_WS_URL || 'ws://localhost:8080/ws';
    
    // For now, we'll use a simple WebSocket connection since we're using Gorilla WebSocket
    this.socket = io(wsUrl.replace('ws://', 'http://').replace('wss://', 'https://'), {
      transports: ['websocket'],
      upgrade: false
    });

    this.socket.on('connect', () => {
      console.log('WebSocket connected');
      this.emit('connected', null);
    });

    this.socket.on('disconnect', () => {
      console.log('WebSocket disconnected');
      this.emit('disconnected', null);
    });

    this.socket.on('message', (message: WebSocketMessage) => {
      console.log('WebSocket message received:', message);
      this.emit(message.type, message.data);
    });

    this.socket.on('error', (error: any) => {
      console.error('WebSocket error:', error);
      this.emit('error', error);
    });
  }

  disconnect(): void {
    if (this.socket) {
      this.socket.disconnect();
      this.socket = null;
    }
  }

  send(type: string, data: any): void {
    if (this.socket?.connected) {
      this.socket.emit('message', {
        type,
        data,
        time: new Date().toISOString()
      });
    }
  }

  subscribe(event: string, callback: (data: any) => void): void {
    if (!this.listeners.has(event)) {
      this.listeners.set(event, []);
    }
    this.listeners.get(event)!.push(callback);

    // Send subscription message to server
    this.send('subscribe', event);
  }

  unsubscribe(event: string, callback: (data: any) => void): void {
    const eventListeners = this.listeners.get(event);
    if (eventListeners) {
      const index = eventListeners.indexOf(callback);
      if (index > -1) {
        eventListeners.splice(index, 1);
      }
    }
  }

  private emit(event: string, data: any): void {
    const eventListeners = this.listeners.get(event);
    if (eventListeners) {
      eventListeners.forEach(callback => callback(data));
    }
  }

  // Price update subscriptions
  subscribeToPriceUpdates(itemId: number, callback: (data: any) => void): void {
    this.subscribe(`price_update_${itemId}`, callback);
  }

  subscribeToArbitrageOpportunities(callback: (data: any) => void): void {
    this.subscribe('arbitrage_opportunities', callback);
  }

  subscribeToTradeUpdates(callback: (data: any) => void): void {
    this.subscribe('trade_updates', callback);
  }

  subscribeToMarketTrends(callback: (data: any) => void): void {
    this.subscribe('market_trends', callback);
  }

  isConnected(): boolean {
    return this.socket?.connected || false;
  }
}

export const websocketService = new WebSocketService();