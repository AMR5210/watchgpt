import AuthenticationServices
import Combine
import CryptoKit
import Foundation
import Security

struct CognitoConfig {
    static let region = "<REGION>"
    static let userPoolID = "<USER_POOL_ID>"
    static let clientID = "<APP_CLIENT_ID>"
    static let hostedDomain = "<HOSTED_DOMAIN>.auth.<REGION>.amazoncognito.com"
    static let redirectURI = "watchgpt://auth"
    static let callbackScheme = "watchgpt"
}

@MainActor
final class CognitoAuthManager: NSObject, ObservableObject {
    static let shared = CognitoAuthManager()

    @Published private(set) var isSignedIn = false
    @Published private(set) var isRefreshing = false

    private let keychain = KeychainStore(service: "personal.watchapp.watchgpt.auth")
    private let tokenKey = "cognito_tokens"
    private var webSession: ASWebAuthenticationSession?

    private override init() {
        super.init()
        isSignedIn = loadTokens() != nil
    }

    func signIn() async throws {
        let verifier = PKCE.makeVerifier()
        let challenge = PKCE.challenge(for: verifier)
        let state = UUID().uuidString
        let callback = try await authenticate(verifier: verifier, challenge: challenge, state: state)
        let parts = URLComponents(url: callback, resolvingAgainstBaseURL: false)
        let returnedState = parts?.queryItems?.first(where: { $0.name == "state" })?.value
        let code = parts?.queryItems?.first(where: { $0.name == "code" })?.value
        let callbackError = parts?.queryItems?.first(where: { $0.name == "error" })?.value
        let callbackErrorDescription = parts?.queryItems?.first(where: { $0.name == "error_description" })?.value

        if let callbackError {
            throw AuthError.callbackError(callbackErrorDescription ?? callbackError)
        }
        guard returnedState == state else { throw AuthError.invalidCallback }
        guard let code, !code.isEmpty else { throw AuthError.missingCode }

        let tokens = try await requestTokens(
            parameters: [
                "grant_type": "authorization_code",
                "client_id": CognitoConfig.clientID,
                "code": code,
                "code_verifier": verifier,
                "redirect_uri": CognitoConfig.redirectURI
            ]
        )

        saveTokens(tokens)
        isSignedIn = true
    }

    func signOut() {
        keychain.delete(tokenKey)
        isSignedIn = false
    }

    func validAccessToken() async throws -> String {
        guard let tokens = loadTokens() else {
            isSignedIn = false
            throw AuthError.notSignedIn
        }

        if tokens.expiresAt > Date().addingTimeInterval(60) {
            return tokens.accessToken
        }

        guard let refreshToken = tokens.refreshToken else {
            signOut()
            throw AuthError.notSignedIn
        }

        isRefreshing = true
        defer { isRefreshing = false }

        let refreshed = try await requestTokens(
            parameters: [
                "grant_type": "refresh_token",
                "client_id": CognitoConfig.clientID,
                "refresh_token": refreshToken
            ],
            existingRefreshToken: refreshToken
        )

        saveTokens(refreshed)
        isSignedIn = true
        return refreshed.accessToken
    }

    private func authenticate(verifier: String, challenge: String, state: String) async throws -> URL {
        var components = URLComponents()
        components.scheme = "https"
        components.host = CognitoConfig.hostedDomain
        components.path = "/oauth2/authorize"
        components.queryItems = [
            URLQueryItem(name: "client_id", value: CognitoConfig.clientID),
            URLQueryItem(name: "response_type", value: "code"),
            URLQueryItem(name: "scope", value: "openid email"),
            URLQueryItem(name: "redirect_uri", value: CognitoConfig.redirectURI),
            URLQueryItem(name: "code_challenge_method", value: "S256"),
            URLQueryItem(name: "code_challenge", value: challenge),
            URLQueryItem(name: "state", value: state)
        ]

        guard let url = components.url else { throw AuthError.invalidURL }

        return try await withCheckedThrowingContinuation { continuation in
            let session = ASWebAuthenticationSession(url: url, callbackURLScheme: CognitoConfig.callbackScheme) { callbackURL, error in
                if let error {
                    continuation.resume(throwing: error)
                    return
                }
                guard let callbackURL else {
                    continuation.resume(throwing: AuthError.invalidCallback)
                    return
                }
                continuation.resume(returning: callbackURL)
            }

            session.prefersEphemeralWebBrowserSession = false
            webSession = session
            if !session.start() {
                continuation.resume(throwing: AuthError.couldNotStartLogin)
            }
        }
    }

    private func requestTokens(parameters: [String: String], existingRefreshToken: String? = nil) async throws -> TokenSet {
        let url = URL(string: "https://\(CognitoConfig.hostedDomain)/oauth2/token")!
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/x-www-form-urlencoded", forHTTPHeaderField: "Content-Type")
        request.httpBody = formBody(parameters)

        let (data, response) = try await URLSession.shared.data(for: request)
        guard let http = response as? HTTPURLResponse else { throw AuthError.invalidResponse }
        guard http.statusCode == 200 else {
            let body = String(data: data, encoding: .utf8) ?? "Token request failed"
            throw AuthError.tokenRequestFailed(body)
        }

        let body = try JSONDecoder().decode(TokenResponse.self, from: data)
        return TokenSet(
            accessToken: body.accessToken,
            refreshToken: body.refreshToken ?? existingRefreshToken,
            idToken: body.idToken,
            expiresAt: Date().addingTimeInterval(TimeInterval(body.expiresIn))
        )
    }

    private func formBody(_ parameters: [String: String]) -> Data {
        let encoded = parameters
            .sorted { $0.key < $1.key }
            .map { key, value in
                "\(key.urlFormEncoded)=\(value.urlFormEncoded)"
            }
            .joined(separator: "&")
        return Data(encoded.utf8)
    }

    private func loadTokens() -> TokenSet? {
        guard let data = keychain.data(for: tokenKey) else { return nil }
        return try? JSONDecoder().decode(TokenSet.self, from: data)
    }

    private func saveTokens(_ tokens: TokenSet) {
        if let data = try? JSONEncoder().encode(tokens) {
            keychain.set(data, for: tokenKey)
        }
    }
}

private struct TokenResponse: Decodable {
    let accessToken: String
    let refreshToken: String?
    let idToken: String?
    let expiresIn: Int

    enum CodingKeys: String, CodingKey {
        case accessToken = "access_token"
        case refreshToken = "refresh_token"
        case idToken = "id_token"
        case expiresIn = "expires_in"
    }
}

private struct TokenSet: Codable {
    let accessToken: String
    let refreshToken: String?
    let idToken: String?
    let expiresAt: Date
}

private enum AuthError: LocalizedError {
    case couldNotStartLogin
    case invalidCallback
    case invalidResponse
    case invalidURL
    case missingCode
    case notSignedIn
    case callbackError(String)
    case tokenRequestFailed(String)

    var errorDescription: String? {
        switch self {
        case .couldNotStartLogin: return "Could not start login."
        case .invalidCallback: return "Invalid login callback."
        case .invalidResponse: return "Invalid auth response."
        case .invalidURL: return "Invalid auth URL."
        case .missingCode: return "Login did not return an authorization code."
        case .notSignedIn: return "Please sign in again."
        case .callbackError(let body): return "Login failed: \(body.prefix(200))"
        case .tokenRequestFailed(let body): return "Auth failed: \(body.prefix(200))"
        }
    }
}

private enum PKCE {
    static func makeVerifier() -> String {
        var bytes = [UInt8](repeating: 0, count: 32)
        _ = SecRandomCopyBytes(kSecRandomDefault, bytes.count, &bytes)
        return Data(bytes).base64URLEncodedString()
    }

    static func challenge(for verifier: String) -> String {
        let digest = SHA256.hash(data: Data(verifier.utf8))
        return Data(digest).base64URLEncodedString()
    }
}

private final class KeychainStore {
    private let service: String

    init(service: String) {
        self.service = service
    }

    func data(for key: String) -> Data? {
        var query = baseQuery(key)
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne

        var result: AnyObject?
        let status = SecItemCopyMatching(query as CFDictionary, &result)
        guard status == errSecSuccess else { return nil }
        return result as? Data
    }

    func set(_ data: Data, for key: String) {
        var query = baseQuery(key)
        let attributes: [String: Any] = [kSecValueData as String: data]

        let status = SecItemUpdate(query as CFDictionary, attributes as CFDictionary)
        if status == errSecItemNotFound {
            query[kSecValueData as String] = data
            query[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
            SecItemAdd(query as CFDictionary, nil)
        }
    }

    func delete(_ key: String) {
        SecItemDelete(baseQuery(key) as CFDictionary)
    }

    private func baseQuery(_ key: String) -> [String: Any] {
        [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: key
        ]
    }
}

private extension Data {
    func base64URLEncodedString() -> String {
        base64EncodedString()
            .replacingOccurrences(of: "+", with: "-")
            .replacingOccurrences(of: "/", with: "_")
            .replacingOccurrences(of: "=", with: "")
    }
}

private extension String {
    var urlFormEncoded: String {
        addingPercentEncoding(withAllowedCharacters: .urlFormAllowed) ?? self
    }
}

private extension CharacterSet {
    static let urlFormAllowed: CharacterSet = {
        var allowed = CharacterSet.urlQueryAllowed
        allowed.remove(charactersIn: ":#[]@!$&'()*+,;=")
        return allowed
    }()
}
