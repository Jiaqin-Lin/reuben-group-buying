import { createContext, useContext, useState, useCallback, type ReactNode } from 'react';

interface AuthState {
  userId: string | null;
  setUserId: (id: string) => void;
  logout: () => void;
  isLoggedIn: boolean;
  isAdmin: boolean;
  setAdminToken: (token: string) => void;
  clearAdmin: () => void;
}

const AuthContext = createContext<AuthState>({
  userId: null,
  setUserId: () => {},
  logout: () => {},
  isLoggedIn: false,
  isAdmin: false,
  setAdminToken: () => {},
  clearAdmin: () => {},
});

// sessionStorage: 每个 tab 独立，方便多用户模拟拼团
const STORAGE_KEY = 'gbm_user_id';
const ADMIN_TOKEN_KEY = 'gbm_admin_token';

function getStoredUserId(): string | null {
  try {
    return sessionStorage.getItem(STORAGE_KEY);
  } catch {
    return null;
  }
}

function storeUserId(id: string) {
  try {
    sessionStorage.setItem(STORAGE_KEY, id);
  } catch {
    // ignore
  }
}

function clearUserId() {
  try {
    sessionStorage.removeItem(STORAGE_KEY);
  } catch {
    // ignore
  }
}

function getStoredAdminToken(): string | null {
  try {
    return sessionStorage.getItem(ADMIN_TOKEN_KEY);
  } catch {
    return null;
  }
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [userId, setUserIdState] = useState<string | null>(getStoredUserId);
  const [isAdmin, setIsAdmin] = useState(() => !!getStoredAdminToken());

  const setUserId = useCallback((id: string) => {
    storeUserId(id);
    setUserIdState(id);
  }, []);

  const logout = useCallback(() => {
    clearUserId();
    setUserIdState(null);
  }, []);

  const setAdminToken = useCallback((token: string) => {
    try {
      sessionStorage.setItem(ADMIN_TOKEN_KEY, token);
    } catch {
      // ignore
    }
    setIsAdmin(true);
  }, []);

  const clearAdmin = useCallback(() => {
    try {
      sessionStorage.removeItem(ADMIN_TOKEN_KEY);
    } catch {
      // ignore
    }
    setIsAdmin(false);
  }, []);

  return (
    <AuthContext.Provider
      value={{ userId, setUserId, logout, isLoggedIn: !!userId, isAdmin, setAdminToken, clearAdmin }}
    >
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  return useContext(AuthContext);
}
