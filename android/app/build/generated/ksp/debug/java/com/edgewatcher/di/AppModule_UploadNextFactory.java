package com.edgewatcher.di;

import com.edgewatcher.domain.port.Clock;
import com.edgewatcher.domain.port.CredentialStore;
import com.edgewatcher.domain.port.ObservationApi;
import com.edgewatcher.domain.port.ObservationBuffer;
import com.edgewatcher.usecase.RefreshSessionUseCase;
import com.edgewatcher.usecase.UploadNextObservationUseCase;
import dagger.internal.DaggerGenerated;
import dagger.internal.Factory;
import dagger.internal.Preconditions;
import dagger.internal.Provider;
import dagger.internal.QualifierMetadata;
import dagger.internal.ScopeMetadata;
import javax.annotation.processing.Generated;

@ScopeMetadata
@QualifierMetadata
@DaggerGenerated
@Generated(
    value = "dagger.internal.codegen.ComponentProcessor",
    comments = "https://dagger.dev"
)
@SuppressWarnings({
    "unchecked",
    "rawtypes",
    "KotlinInternal",
    "KotlinInternalInJava",
    "cast",
    "deprecation",
    "nullness:initialization.field.uninitialized"
})
public final class AppModule_UploadNextFactory implements Factory<UploadNextObservationUseCase> {
  private final Provider<ObservationApi> apiProvider;

  private final Provider<CredentialStore> storeProvider;

  private final Provider<ObservationBuffer> bufferProvider;

  private final Provider<Clock> clockProvider;

  private final Provider<RefreshSessionUseCase> refreshSessionProvider;

  private AppModule_UploadNextFactory(Provider<ObservationApi> apiProvider,
      Provider<CredentialStore> storeProvider, Provider<ObservationBuffer> bufferProvider,
      Provider<Clock> clockProvider, Provider<RefreshSessionUseCase> refreshSessionProvider) {
    this.apiProvider = apiProvider;
    this.storeProvider = storeProvider;
    this.bufferProvider = bufferProvider;
    this.clockProvider = clockProvider;
    this.refreshSessionProvider = refreshSessionProvider;
  }

  @Override
  public UploadNextObservationUseCase get() {
    return uploadNext(apiProvider.get(), storeProvider.get(), bufferProvider.get(), clockProvider.get(), refreshSessionProvider.get());
  }

  public static AppModule_UploadNextFactory create(Provider<ObservationApi> apiProvider,
      Provider<CredentialStore> storeProvider, Provider<ObservationBuffer> bufferProvider,
      Provider<Clock> clockProvider, Provider<RefreshSessionUseCase> refreshSessionProvider) {
    return new AppModule_UploadNextFactory(apiProvider, storeProvider, bufferProvider, clockProvider, refreshSessionProvider);
  }

  public static UploadNextObservationUseCase uploadNext(ObservationApi api, CredentialStore store,
      ObservationBuffer buffer, Clock clock, RefreshSessionUseCase refreshSession) {
    return Preconditions.checkNotNullFromProvides(AppModule.INSTANCE.uploadNext(api, store, buffer, clock, refreshSession));
  }
}
