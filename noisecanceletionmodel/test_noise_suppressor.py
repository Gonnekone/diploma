import os

import matplotlib.pyplot as plt
import torch
import torchaudio
from torch.utils.data import Dataset, DataLoader
from torchaudio.transforms import Resample

SAMPLE_RATE = 48000
FRAME_LENGTH = 512
HOP_LENGTH = 128
TARGET_LEN = SAMPLE_RATE * 3

MODEL_PATH = "modeldata/noise_suppressor.pth"

CLEAN_DIR = "./data/speech-noise-kaggle/speech-noise-dataset/clean_speech"
NOISY_DIR = "./data/speech-noise-kaggle/speech-noise-dataset/noisy_speech"

OUTPUT_DIR = "./test_results"
os.makedirs(OUTPUT_DIR, exist_ok=True)

device = torch.device(
    "mps"
    if torch.backends.mps.is_available()
    else "cuda"
    if torch.cuda.is_available()
    else "cpu"
)

print("Using device:", device)

window = torch.hann_window(FRAME_LENGTH).to(device)


class TestDataset(Dataset):

    def __init__(self):

        clean_files = sorted([
            os.path.join(CLEAN_DIR, f)
            for f in os.listdir(CLEAN_DIR)
            if f.endswith(".wav")
        ])

        noisy_files = sorted([
            os.path.join(NOISY_DIR, f)
            for f in os.listdir(NOISY_DIR)
            if f.endswith(".wav")
        ])

        split = int(len(clean_files) * 0.9)

        self.clean_files = clean_files[split:]
        self.noisy_files = noisy_files[split:]

    def load_audio(self, path):

        audio, sr = torchaudio.load(path)

        if sr != SAMPLE_RATE:
            audio = Resample(sr, SAMPLE_RATE)(audio)

        if audio.size(1) > TARGET_LEN:
            audio = audio[:, :TARGET_LEN]
        else:
            audio = torch.nn.functional.pad(audio, (0, TARGET_LEN - audio.size(1)))

        return audio

    def __len__(self):
        return len(self.clean_files)

    def __getitem__(self, idx):
        clean = self.load_audio(self.clean_files[idx])
        noisy = self.load_audio(self.noisy_files[idx])
        return clean, noisy


class NoiseSuppressor(torch.nn.Module):
    def __init__(self):
        super().__init__()

        freq = FRAME_LENGTH // 2 + 1

        self.encoder = torch.nn.Sequential(
            torch.nn.Conv2d(1, 32, 3, padding=1),
            torch.nn.ReLU(),
            torch.nn.Conv2d(32, 32, 3, padding=1),
            torch.nn.ReLU(),
        )

        self.gru = torch.nn.GRU(
            input_size=32 * freq,
            hidden_size=128,
            batch_first=True
        )

        self.decoder = torch.nn.Sequential(
            torch.nn.Linear(128, freq),
            torch.nn.Sigmoid()
        )

    def forward(self, x):
        x = self.encoder(x)

        B, C, F, T = x.shape

        x = x.permute(0, 3, 1, 2).reshape(B, T, C * F)

        x, _ = self.gru(x)

        mask = self.decoder(x).permute(0, 2, 1)

        return mask


def compute_snr(clean, pred):
    noise = pred - clean
    return 10 * torch.log10(
        torch.mean(clean ** 2) / (torch.mean(noise ** 2) + 1e-6)
    )


def test():
    dataset = TestDataset()
    loader = DataLoader(dataset, batch_size=1, shuffle=False)

    model = NoiseSuppressor().to(device)
    model.load_state_dict(torch.load(MODEL_PATH, map_location=device))

    model.eval()

    total_mse = 0
    total_snr = 0

    print("\nTesting model...\n")

    for i, (clean, noisy) in enumerate(loader):

        clean = clean.to(device)
        noisy = noisy.to(device)

        noisy_complex = torch.stft(
            noisy.squeeze(1),
            n_fft=FRAME_LENGTH,
            hop_length=HOP_LENGTH,
            window=window,
            return_complex=True
        )

        phase = torch.angle(noisy_complex)
        noisy_mag = torch.abs(noisy_complex)

        log_mag = torch.log(noisy_mag + 1e-6).unsqueeze(1)

        with torch.no_grad():
            mask = model(log_mag)

        mask = mask.squeeze(0)

        pred_mag = noisy_mag * mask

        pred_complex = pred_mag * torch.exp(1j * phase)

        audio = torch.istft(
            pred_complex,
            n_fft=FRAME_LENGTH,
            hop_length=HOP_LENGTH,
            window=window,
            length=TARGET_LEN
        )

        audio = audio.unsqueeze(0)

        mse = torch.mean((audio - clean) ** 2).item()
        snr = compute_snr(clean, audio).item()

        total_mse += mse
        total_snr += snr

        if i < 5:
            torchaudio.save(f"{OUTPUT_DIR}/sample_{i}_clean.wav", clean.squeeze(0).cpu(), SAMPLE_RATE)
            torchaudio.save(f"{OUTPUT_DIR}/sample_{i}_noisy.wav", noisy.squeeze(0).cpu(), SAMPLE_RATE)
            torchaudio.save(f"{OUTPUT_DIR}/sample_{i}_denoised.wav", audio.squeeze(0).cpu(), SAMPLE_RATE)

            print(f"Saved sample {i}")

            clean_mag = torch.abs(torch.stft(
                clean.squeeze(1), n_fft=FRAME_LENGTH, hop_length=HOP_LENGTH,
                window=window, return_complex=True
            )).squeeze(0).cpu()

            noisy_mag_plot = noisy_mag.squeeze(0).cpu()
            pred_mag_plot = pred_mag.squeeze(0).cpu()

            # общий диапазон для честного сравнения
            vmin = min(clean_mag.min(), noisy_mag_plot.min(), pred_mag_plot.min()).item()
            vmax = max(clean_mag.max(), noisy_mag_plot.max(), pred_mag_plot.max()).item()

            frames_per_sec = SAMPLE_RATE / HOP_LENGTH
            duration = clean_mag.shape[1] / frames_per_sec

            fig, axes = plt.subplots(3, 1, figsize=(12, 12))

            for ax, data, title in zip(
                    axes,
                    [noisy_mag_plot, clean_mag, pred_mag_plot],
                    ["Зашумлённый сигнал", "Чистый сигнал", "Восстановленный сигнал"]
            ):
                im = ax.imshow(
                    data.numpy(), origin="lower", aspect="auto",
                    vmin=vmin, vmax=vmax,
                    extent=[0, duration, 0, SAMPLE_RATE // 2]
                )
                ax.set_title(title)
                ax.set_xlabel("Время (сек)")
                ax.set_ylabel("Частота (Гц)")
                ax.set_ylim(0, 8000)

            plt.suptitle(f"Sample {i}")
            plt.tight_layout()
            plt.savefig(f"{OUTPUT_DIR}/sample_{i}_spectrograms.png")
            plt.close()

            print(f"Saved spectrograms for sample {i}")

    print("\n====================")
    print("RESULTS")
    print("====================\n")

    print("Average MSE:", total_mse / len(loader))
    print("Average SNR:", total_snr / len(loader))


if __name__ == "__main__":
    test()
