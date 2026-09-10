package com.edgewatcher.di;

import com.edgewatcher.domain.port.CredentialStore;
import com.edgewatcher.domain.port.ObservationApi;
import com.edgewatcher.usecase.PairDeviceUseCase;
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
public final class AppModule_PairDeviceFactory implements Factory<PairDeviceUseCase> {
  private final Provider<ObservationApi> apiProvider;

  private final Provider<CredentialStore> storeProvider;

  private AppModule_PairDeviceFactory(Provider<ObservationApi> apiProvider,
      Provider<CredentialStore> storeProvider) {
    this.apiProvider = apiProvider;
    this.storeProvider = storeProvider;
  }

  @Override
  public PairDeviceUseCase get() {
    return pairDevice(apiProvider.get(), storeProvider.get());
  }

  public static AppModule_PairDeviceFactory create(Provider<ObservationApi> apiProvider,
      Provider<CredentialStore> storeProvider) {
    return new AppModule_PairDeviceFactory(apiProvider, storeProvider);
  }

  public static PairDeviceUseCase pairDevice(ObservationApi api, CredentialStore store) {
    return Preconditions.checkNotNullFromProvides(AppModule.INSTANCE.pairDevice(api, store));
  }
}
