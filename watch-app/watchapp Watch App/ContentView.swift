import SwiftUI
import PhotosUI

// MARK: - Chat Message Model

struct ChatMessage: Identifiable {
    let id = UUID()
    let role: String
    let text: String
    let imageData: Data?

    var isUser: Bool { role == "user" }
}

// MARK: - Main Tab View

struct ContentView: View {
    @StateObject private var auth = CognitoAuthManager.shared

    var body: some View {
        Group {
            if auth.isSignedIn {
                MainTabView()
                    .environmentObject(auth)
            } else {
                LoginView()
                    .environmentObject(auth)
            }
        }
    }
}

struct MainTabView: View {
    var body: some View {
        TabView {
            QuickAnalyzeView()
                .tabItem { Label("Quick", systemImage: "bolt") }

            TextChatView()
                .tabItem { Label("Chat", systemImage: "message") }

            ImageChatView()
                .tabItem { Label("Photo", systemImage: "photo") }
        }
    }
}

struct LoginView: View {
    @EnvironmentObject private var auth: CognitoAuthManager
    @State private var isSigningIn = false
    @State private var errorMessage = ""

    var body: some View {
        NavigationStack {
            VStack(spacing: 12) {
                Image(systemName: "lock.shield")
                    .font(.largeTitle)
                    .foregroundStyle(.blue)

                Text("WatchGPT")
                    .font(.headline)

                Text("Sign in to use your private AI backend.")
                    .font(.caption2)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)

                Button {
                    Task { await signIn() }
                } label: {
                    if isSigningIn {
                        ProgressView()
                            .frame(maxWidth: .infinity)
                    } else {
                        Label("Sign In", systemImage: "person.crop.circle")
                            .frame(maxWidth: .infinity)
                    }
                }
                .buttonStyle(.borderedProminent)
                .disabled(isSigningIn)

                if !errorMessage.isEmpty {
                    Text(errorMessage)
                        .font(.caption2)
                        .foregroundStyle(.red)
                        .multilineTextAlignment(.center)
                }
            }
            .padding(.horizontal, 6)
            .navigationTitle("Login")
        }
    }

    private func signIn() async {
        isSigningIn = true
        errorMessage = ""
        defer { isSigningIn = false }

        do {
            try await auth.signIn()
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}

// MARK: - Quick Analyze View (demo - pick photo, get answer)

struct QuickAnalyzeView: View {
    @EnvironmentObject private var auth: CognitoAuthManager
    @State private var selectedItem: PhotosPickerItem?
    @State private var imageData: Data?
    @State private var hasImage: Bool = false
    @State private var result: String = ""
    @State private var isLoading: Bool = false

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 12) {
                    EmptyStateView(
                        icon: hasImage ? "checkmark.circle.fill" : "photo.on.rectangle",
                        title: hasImage ? "Photo ready" : "Quick analyze",
                        message: hasImage ? "Tap Analyze to send it." : "Pick a photo or run the demo."
                    )

                    Button {
                        auth.signOut()
                    } label: {
                        Label("Sign Out", systemImage: "rectangle.portrait.and.arrow.right")
                    }
                    .buttonStyle(.plain)
                    .font(.caption2)
                    .foregroundStyle(.secondary)

                    PhotosPicker(selection: $selectedItem, matching: .images) {
                        HStack {
                            Image(systemName: hasImage ? "checkmark.circle.fill" : "photo.on.rectangle")
                                .foregroundStyle(hasImage ? .green : .blue)
                            Text(hasImage ? "Photo selected" : "Pick Photo")
                        }
                    }

                    Button {
                        Task { await analyze() }
                    } label: {
                        if isLoading {
                            ProgressView()
                                .frame(maxWidth: .infinity)
                        } else {
                            Text("Analyze")
                                .frame(maxWidth: .infinity)
                        }
                    }
                    .buttonStyle(.borderedProminent)
                    .disabled(imageData == nil || isLoading)

                    Button {
                        Task { await runDemo() }
                    } label: {
                        HStack {
                            Image(systemName: "play.fill")
                            Text("Try Demo")
                        }
                        .frame(maxWidth: .infinity)
                    }
                    .buttonStyle(.bordered)
                    .disabled(isLoading)

                    if !result.isEmpty {
                        Divider()

                        Text(result)
                            .font(.footnote)
                            .padding(10)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .background(Color.gray.opacity(0.16))
                            .clipShape(RoundedRectangle(cornerRadius: 10))

                        Button {
                            result = ""
                            selectedItem = nil
                            imageData = nil
                            hasImage = false
                        } label: {
                            Label("Clear", systemImage: "xmark.circle")
                        }
                        .buttonStyle(.plain)
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                        .padding(.top, 2)
                    }
                }
                .padding(.horizontal, 4)
            }
            .navigationTitle("WatchGPT")
        }
        .onChange(of: selectedItem) { _, newItem in
            Task {
                if let data = try? await newItem?.loadTransferable(type: Data.self) {
                    imageData = data
                    hasImage = true
                } else {
                    imageData = nil
                    hasImage = false
                }
            }
        }
    }

    private func analyze() async {
        guard let imageData else { return }
        isLoading = true
        result = ""
        defer { isLoading = false }

        do {
            result = try await OpenAIClient.shared.analyze(imageData: imageData)
        } catch {
            result = "Error: \(error.localizedDescription)"
        }
    }

    private func runDemo() async {
        if let uiImage = UIImage(named: "demo_image"),
           let demoData = uiImage.jpegData(compressionQuality: 0.8) {
            imageData = demoData
            hasImage = true
        }

        guard let imageData else { return }
        isLoading = true
        result = ""
        defer { isLoading = false }

        do {
            result = try await OpenAIClient.shared.analyze(imageData: imageData, prompt: "Describe what you see in the image.")
        } catch {
            result = "Error: \(error.localizedDescription)"
        }
    }
}

// MARK: - Text Chat View (no images)

struct TextChatView: View {
    @State private var messages: [ChatMessage] = []
    @State private var inputText: String = ""
    @State private var isLoading: Bool = false
    @State private var streamingText: String = ""
    @State private var isInputVisible: Bool = false

    var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                ScrollViewReader { scrollProxy in
                    ScrollView {
                        LazyVStack(alignment: .leading, spacing: 8) {
                            if messages.isEmpty && streamingText.isEmpty && !isLoading {
                                EmptyStateView(
                                    icon: "message",
                                    title: "Start a chat",
                                    message: "Ask a short question or paste text."
                                )
                            }

                            ForEach(messages) { msg in
                                MessageBubble(message: msg)
                                    .id(msg.id)
                            }
                            if !streamingText.isEmpty {
                                MessageBubble(message: ChatMessage(role: "assistant", text: streamingText, imageData: nil))
                                .id("streaming")
                            }
                            if isLoading && streamingText.isEmpty {
                                LoadingRow(text: "Connecting...")
                                    .id("loading")
                            }
                        }
                        .padding(.horizontal, 5)
                        .padding(.top, 4)
                    }
                    .onChange(of: streamingText) { _, _ in
                        withAnimation {
                            scrollProxy.scrollTo("streaming", anchor: .bottom)
                        }
                    }
                    .onChange(of: messages.count) { _, _ in
                        if let last = messages.last {
                            withAnimation {
                                scrollProxy.scrollTo(last.id, anchor: .bottom)
                            }
                        }
                    }
                }

                if isInputVisible {
                    ChatInputBar(
                        text: $inputText,
                        placeholder: "Ask...",
                        isLoading: isLoading,
                        isSendDisabled: inputText.trimmingCharacters(in: .whitespaces).isEmpty,
                        onCollapse: {
                            isInputVisible = false
                        }
                    ) {
                        Task { await sendMessage() }
                    }
                } else {
                    CollapsedInputBar {
                        isInputVisible = true
                    }
                }
            }
            .navigationTitle("Chat")
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        messages = []
                        inputText = ""
                        streamingText = ""
                        isInputVisible = false
                    } label: {
                        Image(systemName: "trash")
                            .font(.caption)
                    }
                }
            }
        }
    }

    private func sendMessage() async {
        let text = inputText.trimmingCharacters(in: .whitespaces)
        guard !text.isEmpty else { return }

        messages.append(ChatMessage(role: "user", text: text, imageData: nil))
        inputText = ""
        isInputVisible = false
        isLoading = true
        streamingText = ""

        do {
            let payloads = messages.map { msg in
                OpenAIClient.ChatMessagePayload(role: msg.role, content: msg.text, image: nil)
            }

            try await OpenAIClient.shared.chatStream(messages: payloads) { token in
                Task { @MainActor in
                    streamingText += token
                }
            }

            // Streaming done — commit to messages
            let finalText = streamingText
            streamingText = ""
            messages.append(ChatMessage(role: "assistant", text: finalText, imageData: nil))
        } catch {
            streamingText = ""
            messages.append(ChatMessage(role: "assistant", text: "Error: \(error.localizedDescription)", imageData: nil))
        }

        isLoading = false
    }
}

// MARK: - Image Chat View (with photo picker)

struct ImageChatView: View {
    @State private var messages: [ChatMessage] = []
    @State private var inputText: String = ""
    @State private var isLoading: Bool = false
    @State private var selectedItem: PhotosPickerItem?
    @State private var attachedImageData: Data?
    @State private var hasAttachment: Bool = false
    @State private var isInputVisible: Bool = false

    private let defaultPrompt = "Analyze the content in this image and answer as briefly as possible."

    var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                ScrollViewReader { scrollProxy in
                    ScrollView {
                        LazyVStack(alignment: .leading, spacing: 8) {
                            if messages.isEmpty && !isLoading {
                                EmptyStateView(
                                    icon: "photo",
                                    title: "Photo chat",
                                    message: "Attach a photo, ask a question, or do both."
                                )
                            }

                            ForEach(messages) { msg in
                                MessageBubble(message: msg)
                                    .id(msg.id)
                            }
                            if isLoading {
                                LoadingRow(text: "Thinking...")
                                    .id("loading")
                            }
                        }
                        .padding(.horizontal, 5)
                        .padding(.top, 4)
                    }
                    .onChange(of: messages.count) { _, _ in
                        if let last = messages.last {
                            withAnimation {
                                scrollProxy.scrollTo(last.id, anchor: .bottom)
                            }
                        }
                    }
                }

                if isInputVisible {
                    ChatInputBar(
                        text: $inputText,
                        placeholder: hasAttachment ? "Add context..." : "Ask...",
                        isLoading: isLoading,
                        isSendDisabled: inputText.trimmingCharacters(in: .whitespaces).isEmpty && attachedImageData == nil,
                        leading: {
                            PhotosPicker(selection: $selectedItem, matching: .images) {
                                Image(systemName: hasAttachment ? "photo.fill" : "photo")
                                    .foregroundStyle(hasAttachment ? .green : .blue)
                                    .font(.body)
                            }
                        },
                        onCollapse: {
                            isInputVisible = false
                        },
                        onSend: {
                            Task { await sendMessage() }
                        }
                    )
                } else {
                    CollapsedInputBar {
                        isInputVisible = true
                    }
                }
            }
            .navigationTitle("Photo")
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        messages = []
                        inputText = ""
                        attachedImageData = nil
                        hasAttachment = false
                        selectedItem = nil
                        isInputVisible = false
                    } label: {
                        Image(systemName: "trash")
                            .font(.caption)
                    }
                }
            }
        }
        .onChange(of: selectedItem) { _, newItem in
            Task {
                if let data = try? await newItem?.loadTransferable(type: Data.self) {
                    attachedImageData = data
                    hasAttachment = true
                } else {
                    attachedImageData = nil
                    hasAttachment = false
                }
            }
        }
    }

    private func sendMessage() async {
        let text = inputText.trimmingCharacters(in: .whitespaces)
        let imageData = attachedImageData

        let messageText = text.isEmpty && imageData != nil ? defaultPrompt : text
        guard !messageText.isEmpty || imageData != nil else { return }

        messages.append(ChatMessage(role: "user", text: text.isEmpty ? "Analyze image" : text, imageData: imageData))

        inputText = ""
        isInputVisible = false
        attachedImageData = nil
        hasAttachment = false
        selectedItem = nil

        isLoading = true
        defer { isLoading = false }

        do {
            var payloads: [OpenAIClient.ChatMessagePayload] = []

            for msg in messages {
                var base64Image: String? = nil
                if let data = msg.imageData {
                    let compressed = OpenAIClient.shared.compressImage(data: data, maxBytes: 500_000)
                    base64Image = compressed.base64EncodedString()
                }
                payloads.append(OpenAIClient.ChatMessagePayload(
                    role: msg.role,
                    content: msg.role == "user" && msg.text == "Analyze image" ? messageText : msg.text,
                    image: base64Image
                ))
            }

            let reply = try await OpenAIClient.shared.chat(messages: payloads)
            messages.append(ChatMessage(role: "assistant", text: reply, imageData: nil))
        } catch {
            messages.append(ChatMessage(role: "assistant", text: "Error: \(error.localizedDescription)", imageData: nil))
        }
    }
}

// MARK: - Message Bubble

struct MessageBubble: View {
    let message: ChatMessage

    var body: some View {
        HStack {
            if message.isUser { Spacer(minLength: 20) }

            VStack(alignment: message.isUser ? .trailing : .leading, spacing: 4) {
                if message.imageData != nil {
                    Label("Photo", systemImage: "photo.fill")
                        .font(.caption2)
                        .foregroundStyle(.green)
                }
                Text(message.text)
                    .font(.caption2)
                    .padding(.horizontal, 9)
                    .padding(.vertical, 6)
                    .background(message.isUser ? Color.blue.opacity(0.32) : Color.gray.opacity(0.16))
                    .clipShape(RoundedRectangle(cornerRadius: 11))
            }

            if !message.isUser { Spacer(minLength: 20) }
        }
    }
}

struct EmptyStateView: View {
    let icon: String
    let title: String
    let message: String

    var body: some View {
        VStack(spacing: 5) {
            Image(systemName: icon)
                .font(.title3)
                .foregroundStyle(.blue)
            Text(title)
                .font(.headline)
            Text(message)
                .font(.caption2)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 10)
    }
}

struct LoadingRow: View {
    let text: String

    var body: some View {
        HStack(spacing: 6) {
            ProgressView()
            Text(text)
                .font(.caption2)
                .foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.vertical, 4)
    }
}

struct ChatInputBar<Leading: View>: View {
    @Binding var text: String
    let placeholder: String
    let isLoading: Bool
    let isSendDisabled: Bool
    let leading: Leading
    let onCollapse: () -> Void
    let onSend: () -> Void

    init(
        text: Binding<String>,
        placeholder: String,
        isLoading: Bool,
        isSendDisabled: Bool,
        @ViewBuilder leading: () -> Leading,
        onCollapse: @escaping () -> Void,
        onSend: @escaping () -> Void
    ) {
        _text = text
        self.placeholder = placeholder
        self.isLoading = isLoading
        self.isSendDisabled = isSendDisabled
        self.leading = leading()
        self.onCollapse = onCollapse
        self.onSend = onSend
    }

    var body: some View {
        HStack(spacing: 6) {
            Button(action: onCollapse) {
                Image(systemName: "chevron.down.circle")
                    .font(.body)
                    .foregroundStyle(.secondary)
            }
            .buttonStyle(.plain)

            leading
                .frame(width: 24, height: 24)

            TextField(placeholder, text: $text)
                .font(.caption2)
                .textFieldStyle(.plain)

            Button(action: onSend) {
                Image(systemName: "arrow.up.circle.fill")
                    .font(.title3)
                    .foregroundStyle(isSendDisabled || isLoading ? Color.secondary : Color.blue)
            }
            .buttonStyle(.plain)
            .disabled(isSendDisabled || isLoading)
        }
        .padding(.horizontal, 7)
        .padding(.vertical, 6)
        .background(Color.gray.opacity(0.14))
        .clipShape(RoundedRectangle(cornerRadius: 12))
        .padding(.horizontal, 4)
        .padding(.top, 4)
        .padding(.bottom, -10)
        .offset(y: 10)
    }
}

extension ChatInputBar where Leading == EmptyView {
    init(
        text: Binding<String>,
        placeholder: String,
        isLoading: Bool,
        isSendDisabled: Bool,
        onCollapse: @escaping () -> Void,
        onSend: @escaping () -> Void
    ) {
        self.init(
            text: text,
            placeholder: placeholder,
            isLoading: isLoading,
            isSendDisabled: isSendDisabled,
            leading: { EmptyView() },
            onCollapse: onCollapse,
            onSend: onSend
        )
    }
}

struct CollapsedInputBar<Leading: View, Trailing: View>: View {
    let leading: Leading
    let trailing: Trailing
    let onCompose: () -> Void

    init(
        @ViewBuilder leading: () -> Leading,
        @ViewBuilder trailing: () -> Trailing,
        onCompose: @escaping () -> Void
    ) {
        self.leading = leading()
        self.trailing = trailing()
        self.onCompose = onCompose
    }

    var body: some View {
        HStack(spacing: 12) {
            leading
                .frame(width: 28, height: 28)

            Spacer(minLength: 0)

            Button(action: onCompose) {
                Image(systemName: "square.and.pencil")
                    .font(.title3)
                    .foregroundStyle(.blue)
            }
            .buttonStyle(.plain)

            trailing
                .frame(width: 28, height: 28)
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 4)
        .padding(.bottom, -12)
        .offset(y: 12)
    }
}

extension CollapsedInputBar where Leading == EmptyView, Trailing == EmptyView {
    init(onCompose: @escaping () -> Void) {
        self.init(
            leading: { EmptyView() },
            trailing: { EmptyView() },
            onCompose: onCompose
        )
    }
}

extension CollapsedInputBar where Trailing == EmptyView {
    init(
        @ViewBuilder leading: () -> Leading,
        onCompose: @escaping () -> Void
    ) {
        self.init(
            leading: leading,
            trailing: { EmptyView() },
            onCompose: onCompose
        )
    }
}

#Preview {
    ContentView()
}
