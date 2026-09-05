namespace ScreenShare.App.Copy;

/// <summary>Status band wording around the send-logs button.</summary>
public static class Reports
{
    /// <summary>
    /// Confirmation after a send, naming the stored bundle so the reader can quote it
    /// to the relay operator.
    /// </summary>
    public static string Sent(string reportId) => $"Logs sent ({reportId})";
}
