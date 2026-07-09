import Foundation
import UIKit

final class OpenAIClient {
    static let shared = OpenAIClient()

    private let backendBaseURL = AppConfig.backendBaseURL

    private init() {}

    // MARK: - Types

    private struct AnalyzeRequest: Encodable {
        let image: String
        let prompt: String
    }

    private struct AnalyzeResponse: Decodable {
        let answer: String
        let cached: Bool?
    }

    struct ChatMessagePayload: Encodable {
        let role: String
        let content: String
        let image: String?
    }

    private struct ChatRequest: Encodable {
        let messages: [ChatMessagePayload]
    }

    private struct ChatResponseBody: Decodable {
        let reply: String
    }

    private struct ErrorBody: Decodable {
        let error: String
    }

    // MARK: - Analyze (with caching)

    func analyze(imageData: Data, prompt: String = "Describe what you see in the image.") async throws -> String {
        let compressed = compressImage(data: imageData, maxBytes: 500_000)
        let base64 = compressed.base64EncodedString()

        let reqBody = AnalyzeRequest(image: base64, prompt: prompt)
        let url = URL(string: "\(backendBaseURL)/api/v1/analyze")!

        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        try await authorize(&request)
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.timeoutInterval = 60
        request.httpBody = try JSONEncoder().encode(reqBody)

        let (data, response) = try await URLSession.shared.data(for: request)
        try validateResponse(data: data, response: response)

        let result = try JSONDecoder().decode(AnalyzeResponse.self, from: data)
        return result.answer.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    // MARK: - Chat (non-streaming)

    func chat(messages: [ChatMessagePayload]) async throws -> String {
        let reqBody = ChatRequest(messages: messages)
        let url = URL(string: "\(backendBaseURL)/api/v1/chat")!

        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        try await authorize(&request)
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.timeoutInterval = 60
        request.httpBody = try JSONEncoder().encode(reqBody)

        let (data, response) = try await URLSession.shared.data(for: request)
        try validateResponse(data: data, response: response)

        let result = try JSONDecoder().decode(ChatResponseBody.self, from: data)
        return result.reply.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    // MARK: - Stream (SSE streaming)

    func chatStream(messages: [ChatMessagePayload], onToken: @escaping (String) -> Void) async throws {
        let reqBody = ChatRequest(messages: messages)
        let url = URL(string: "\(backendBaseURL)/api/v1/stream")!

        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        try await authorize(&request)
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.timeoutInterval = 60
        request.httpBody = try JSONEncoder().encode(reqBody)

        let (bytes, response) = try await URLSession.shared.bytes(for: request)

        guard let http = response as? HTTPURLResponse, http.statusCode == 200 else {
            throw APIError.invalidResponse
        }

        for try await line in bytes.lines {
            if line.hasPrefix("data: ") {
                let token = String(line.dropFirst(6))
                if token == "[DONE]" { break }
                if token.hasPrefix("[ERROR]") {
                    throw APIError.httpError(statusCode: 502, body: token)
                }
                onToken(token)
            }
        }
    }

    // MARK: - Helpers

    private func authorize(_ request: inout URLRequest) async throws {
        let token = try await CognitoAuthManager.shared.validAccessToken()
        request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
    }

    private func validateResponse(data: Data, response: URLResponse) throws {
        guard let http = response as? HTTPURLResponse else {
            throw APIError.invalidResponse
        }
        guard http.statusCode == 200 else {
            if let errorBody = try? JSONDecoder().decode(ErrorBody.self, from: data) {
                throw APIError.httpError(statusCode: http.statusCode, body: errorBody.error)
            }
            let message = String(data: data, encoding: .utf8) ?? "Unknown error"
            throw APIError.httpError(statusCode: http.statusCode, body: message)
        }
    }

    func compressImage(data: Data, maxBytes: Int) -> Data {
        guard let image = UIImage(data: data) else { return data }

        var quality: CGFloat = 0.7
        while quality > 0.1 {
            if let compressed = image.jpegData(compressionQuality: quality),
               compressed.count <= maxBytes {
                return compressed
            }
            quality -= 0.1
        }

        let scale = 0.5
        let newSize = CGSize(width: image.size.width * scale, height: image.size.height * scale)
        guard let cgImage = image.cgImage,
              let context = CGContext(
                  data: nil,
                  width: Int(newSize.width),
                  height: Int(newSize.height),
                  bitsPerComponent: 8,
                  bytesPerRow: 0,
                  space: CGColorSpaceCreateDeviceRGB(),
                  bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue
              ) else { return data }
        context.draw(cgImage, in: CGRect(origin: .zero, size: newSize))
        guard let resizedCG = context.makeImage() else { return data }
        return UIImage(cgImage: resizedCG).jpegData(compressionQuality: 0.5) ?? data
    }

    enum APIError: LocalizedError {
        case invalidResponse
        case httpError(statusCode: Int, body: String)
        case parsingFailed

        var errorDescription: String? {
            switch self {
            case .invalidResponse: return "Invalid response from server."
            case .httpError(let code, let body):
                if code == 401 { return "Please sign in again." }
                if code == 429 { return "Rate limited. Try again later." }
                return "HTTP \(code): \(body.prefix(200))"
            case .parsingFailed: return "Could not parse the response."
            }
        }
    }
}
