import os
import random
import time

import matplotlib.pyplot as plt
import torch
import torch.nn as nn
import torch.optim as optim
import torchaudio
import torchaudio.functional as F
from torch.utils.data import Dataset, DataLoader
from torchaudio.transforms import Spectrogram, Resample
from tqdm import tqdm

SAMPLE_RATE = 48000
FRAME_LENGTH = 512
HOP_LENGTH = 128

EPOCHS = 40
BATCH_SIZE = 8
LR = 3e-4

TARGET_LEN = SAMPLE_RATE * 3
SNR_LEVELS = [0, 5, 10, 15, 20]

PLOT_BY = "epoch"

CLEAN_DIR = "./data/speech-noise-kaggle/speech-noise-dataset/clean_speech"
NOISE_DIR = "./data/speech-noise-kaggle/speech-noise-dataset/noise_only"
NOISY_DIR = "./data/speech-noise-kaggle/speech-noise-dataset/noisy_speech"

os.makedirs("modeldata", exist_ok=True)

device = torch.device(
    "mps" if torch.backends.mps.is_available()
    else "cuda" if torch.cuda.is_available()
    else "cpu"
)

print("Using device:", device)


class AudioDataset(Dataset):
    def __init__(self, mode="train"):

        clean = sorted([os.path.join(CLEAN_DIR, f) for f in os.listdir(CLEAN_DIR) if f.endswith(".wav")])
        noisy = sorted([os.path.join(NOISY_DIR, f) for f in os.listdir(NOISY_DIR) if f.endswith(".wav")])
        self.noise = [os.path.join(NOISE_DIR, f) for f in os.listdir(NOISE_DIR) if f.endswith(".wav")]

        split = int(len(clean) * 0.9)

        if mode == "train":
            self.clean = clean[:split]
            self.noisy = noisy[:split]
        else:
            self.clean = clean[split:]
            self.noisy = noisy[split:]

        self.spec = Spectrogram(
            n_fft=FRAME_LENGTH,
            hop_length=HOP_LENGTH,
            power=None
        )

    def load(self, path):
        x, sr = torchaudio.load(path)

        if sr != SAMPLE_RATE:
            x = Resample(sr, SAMPLE_RATE)(x)

        if x.size(1) > TARGET_LEN:
            x = x[:, :TARGET_LEN]
        else:
            x = torch.nn.functional.pad(x, (0, TARGET_LEN - x.size(1)))

        return x

    def __len__(self):
        return len(self.clean) * 5

    def __getitem__(self, idx):

        if random.random() < 0.5:
            i = idx % len(self.clean)
            clean = self.load(self.clean[i])
            noisy = self.load(self.noisy[i])
        else:
            clean = self.load(random.choice(self.clean))
            noise = self.load(random.choice(self.noise))
            snr = random.choice(SNR_LEVELS)
            noisy = F.add_noise(clean, noise, torch.tensor([snr]))

        clean_stft = self.spec(clean)
        noisy_stft = self.spec(noisy)

        clean_mag = torch.abs(clean_stft)
        noisy_mag = torch.abs(noisy_stft)

        clean_log = torch.log(clean_mag + 1e-6)
        noisy_log = torch.log(noisy_mag + 1e-6)

        return noisy_log, clean_log


class NoiseSuppressor(nn.Module):
    def __init__(self):
        super().__init__()
        freq = FRAME_LENGTH // 2 + 1

        self.encoder = nn.Sequential(
            nn.Conv2d(1, 32, 3, padding=1),
            nn.ReLU(),
            nn.Conv2d(32, 32, 3, padding=1),
            nn.ReLU(),
        )

        self.gru = nn.GRU(
            input_size=32 * freq,
            hidden_size=128,
            num_layers=1,
            batch_first=True
        )

        self.decoder = nn.Sequential(
            nn.Linear(128, freq),
            nn.Sigmoid()
        )

    def forward(self, x):
        x = self.encoder(x)

        B, C, F, T = x.shape

        x = x.permute(0, 3, 1, 2).reshape(B, T, C * F)

        x, _ = self.gru(x)

        mask = self.decoder(x).permute(0, 2, 1)

        return mask


def loss_fn(noisy_log, clean_log, mask, noisy_wave, clean_wave, pred_wave):
    noisy_mag = torch.exp(noisy_log.squeeze(1))
    clean_mag = torch.exp(clean_log.squeeze(1))

    target_mask = torch.clamp(clean_mag / (noisy_mag + 1e-6), 0, 1)
    loss_mask = torch.mean((mask - target_mask) ** 2)

    pred_clean_mag = noisy_mag * mask
    loss_spec = torch.mean((pred_clean_mag - clean_mag) ** 2)

    snr_loss = -torch.mean(si_snr(clean_wave, pred_wave))

    return loss_mask + 0.3 * loss_spec + 0.5 * snr_loss


def si_snr(clean, pred):
    clean = clean - clean.mean(dim=-1, keepdim=True)
    pred = pred - pred.mean(dim=-1, keepdim=True)

    s_target = (torch.sum(pred * clean, dim=-1, keepdim=True) * clean) / (
            torch.sum(clean ** 2, dim=-1, keepdim=True) + 1e-6
    )

    e_noise = pred - s_target

    return 10 * torch.log10(
        torch.sum(s_target ** 2, dim=-1) /
        (torch.sum(e_noise ** 2, dim=-1) + 1e-6)
    )


def train():
    ds = AudioDataset("train")
    dl = DataLoader(ds, batch_size=BATCH_SIZE, shuffle=True)

    model = NoiseSuppressor().to(device)

    optimizer = optim.Adam(model.parameters(), lr=LR)
    scheduler = optim.lr_scheduler.ReduceLROnPlateau(
        optimizer, patience=3, factor=0.5
    )

    window = torch.hann_window(FRAME_LENGTH).to(device)

    losses = []
    epoch_losses = []
    mask_losses = []
    spec_losses = []
    snr_losses = []

    print("\nTraining started\n")

    for epoch in range(EPOCHS):

        model.train()
        start = time.time()

        total_loss = 0

        progress = tqdm(dl)

        for noisy_log, clean_log in progress:

            noisy_log = noisy_log.to(device)
            clean_log = clean_log.to(device)

            noisy_mag = torch.exp(noisy_log.squeeze(1))
            clean_mag = torch.exp(clean_log.squeeze(1))

            phase = torch.zeros_like(noisy_mag)

            optimizer.zero_grad()

            mask = model(noisy_log)

            pred_mag = noisy_mag * mask

            pred_complex = pred_mag * torch.exp(1j * phase)

            pred_wave = torch.istft(
                pred_complex,
                n_fft=FRAME_LENGTH,
                hop_length=HOP_LENGTH,
                window=window,
                length=TARGET_LEN
            )

            clean_complex = clean_mag * torch.exp(1j * phase)

            clean_wave = torch.istft(
                clean_complex,
                n_fft=FRAME_LENGTH,
                hop_length=HOP_LENGTH,
                window=window,
                length=TARGET_LEN
            )

            target_mask = torch.clamp(
                clean_mag / (noisy_mag + 1e-6),
                0, 1
            )

            loss_mask = torch.mean((mask - target_mask) ** 2)

            loss_spec = torch.mean((pred_mag - clean_mag) ** 2)

            loss_snr = -torch.mean(si_snr(clean_wave, pred_wave))

            mask_losses.append(loss_mask.item())
            spec_losses.append(loss_spec.item())
            snr_losses.append(-loss_snr.item())

            loss = loss_mask + 0.3 * loss_spec + 0.5 * loss_snr

            loss.backward()

            torch.nn.utils.clip_grad_norm_(model.parameters(), 5.0)

            optimizer.step()

            if PLOT_BY == "batch":
                losses.append(loss.item())

            total_loss += loss.item()

            progress.set_description(f"Epoch {epoch + 1}")
            progress.set_postfix(loss=loss.item())

        avg = total_loss / len(dl)

        if PLOT_BY == "epoch":
            epoch_losses.append(avg)

        scheduler.step(avg)

        print(f"Epoch {epoch + 1} | loss {avg:.4f} | time {time.time() - start:.1f}s")

    torch.save(model.state_dict(), "modeldata/noise_suppressor.pth")
    print("\nModel saved")

    plot_data = epoch_losses if PLOT_BY == "epoch" else losses

    plt.figure(figsize=(12, 6))
    plt.plot(plot_data)
    plt.title("Training Loss")
    plt.xlabel("Epoch" if PLOT_BY == "epoch" else "Batch")
    plt.ylabel("Loss")
    plt.grid()
    plt.savefig("modeldata/training_loss.png")

    print("Loss graph saved")

    model.eval().cpu()

    dummy = torch.randn(
        1,
        1,
        FRAME_LENGTH // 2 + 1,
        100
    )

    torch.onnx.export(
        model,
        dummy,
        "modeldata/noise_suppressor.onnx",
        opset_version=17
    )

    print("ONNX exported")


if __name__ == "__main__":
    train()
