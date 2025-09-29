import axios from 'axios';

// 明确设置 axios 的 baseURL，一处配置即可，避免重复拼接
const API_BASE_URL = process.env.REACT_APP_API_BASE_URL || '/api/v1';
axios.defaults.baseURL = API_BASE_URL;

export interface User {
  id: number;
  steam_id: string;
  username: string;
  avatar: string;
  created_at: string;
  updated_at: string;
}

export interface LoginResponse {
  login_url: string;
}

class AuthService {
  private baseURL: string;

  constructor() {
    // 只使用资源路径，由 axios.defaults.baseURL 统一前缀 '/api/v1'
    this.baseURL = `/auth`;
  }

  async getSteamLoginUrl(returnUrl?: string): Promise<LoginResponse> {
    const response = await axios.get(`${this.baseURL}/steam/login`, {
      params: { return_url: returnUrl }
    });
    return response.data;
  }

  async handleSteamCallback(params: URLSearchParams): Promise<{ user: User; token: string }> {
    const response = await axios.get(`${this.baseURL}/steam/callback?${params.toString()}`);
    const { token, user } = response.data;
    
    // 保存token到localStorage
    this.setAuthToken(token);
    
    return { user, token };
  }

  async getCurrentUser(): Promise<User> {
    const response = await axios.get(`${this.baseURL}/me`);
    return response.data.user;
  }

  async logout(): Promise<void> {
    await axios.post(`${this.baseURL}/logout`);
    this.removeAuthToken();
  }

  isAuthenticated(): boolean {
    // Check if user has valid session/token
    return localStorage.getItem('auth_token') !== null;
  }

  setAuthToken(token: string): void {
    localStorage.setItem('auth_token', token);
    axios.defaults.headers.common['Authorization'] = `Bearer ${token}`;
  }

  removeAuthToken(): void {
    localStorage.removeItem('auth_token');
    delete axios.defaults.headers.common['Authorization'];
  }

  getAuthToken(): string | null {
    return localStorage.getItem('auth_token');
  }
}

export const authService = new AuthService();