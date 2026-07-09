import Foundation

enum AppConfig {
    // Local development: "http://<your-machine-ip>:8080"
    // EKS deployment:    "http://<your-elb-hostname>.amazonaws.com"
    static let backendBaseURL = "http://localhost:8080"
}
